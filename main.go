package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-steputils/output"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/stringutil"
	"github.com/bitrise-io/go-xcode/certificateutil"
	"github.com/bitrise-io/go-xcode/export"
	"github.com/bitrise-io/go-xcode/exportoptions"
	"github.com/bitrise-io/go-xcode/profileutil"
	"github.com/bitrise-io/go-xcode/utility"
	"github.com/bitrise-io/go-xcode/xcarchive"
	"github.com/bitrise-io/go-xcode/xcodebuild"
	"github.com/bitrise-io/go-xcode/xcpretty"
)

const (
	bitriseXcodeRawResultTextEnvKey     = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
	bitriseExportedFilePath             = "BITRISE_EXPORTED_FILE_PATH"
	bitriseDSYMDirPthEnvKey             = "BITRISE_DSYM_PATH"
	bitriseXCArchivePthEnvKey           = "BITRISE_XCARCHIVE_PATH"
	bitriseXCArchiveDirPthEnvKey        = "BITRISE_MACOS_XCARCHIVE_PATH"
	bitriseAppPthEnvKey                 = "BITRISE_APP_PATH"
	bitriseIDEDistributionLogsPthEnvKey = "BITRISE_IDEDISTRIBUTION_LOGS_PATH"
)

// config ...
type config struct {
	ExportMethod                    string `env:"export_method,opt[none,app-store,development,developer-id]"`
	CustomExportOptionsPlistContent string `env:"custom_export_options_plist_content"`

	ProjectPath               string `env:"project_path,dir"`
	Scheme                    string `env:"scheme,required"`
	Configuration             string `env:"configuration"`
	IsCleanBuild              string `env:"is_clean_build,opt[yes,no]"`
	WorkDir                   string `env:"workdir"`
	DisableIndexWhileBuilding bool   `env:"disable_index_while_building,opt[yes,no]"`

	ForceTeamID                       string `env:"force_team_id"`
	ForceCodeSignIdentity             string `env:"force_code_sign_identity"`
	ForceProvisioningProfileSpecifier string `env:"force_provisioning_profile_specifier"`
	ForceProvisioningProfile          string `env:"force_provisioning_profile"`

	OutputTool           string `env:"output_tool,opt[xcpretty,xcodebuild]"`
	OutputDir            string `env:"output_dir,dir"`
	ArtifactName         string `env:"artifact_name,required"`
	IsExportXcarchiveZip string `env:"is_export_xcarchive_zip,opt[yes,no]"`
	IsExportAllDsyms     string `env:"is_export_all_dsyms,opt[yes,no]"`
	VerboseLog           string `env:"verbose_log"`
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

func getXcprettyVersion() (string, error) {
	cmd := command.New("xcpretty", "-version")
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

func findIDEDistrubutionLogsPath(output string) (string, error) {
	pattern := `IDEDistribution: -\[IDEDistributionLogging _createLoggingBundleAtPath:\]: Created bundle at path '(?P<log_path>.*)'`
	re := regexp.MustCompile(pattern)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if match := re.FindStringSubmatch(line); len(match) == 2 {
			return match[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

func macCodeSignGroup(archive xcarchive.MacosArchive, installedCertificates []certificateutil.CertificateInfoModel, installedInstallerCertificates []certificateutil.CertificateInfoModel, installedProfiles []profileutil.ProvisioningProfileInfoModel, exportMethod exportoptions.Method) (*export.MacCodeSignGroup, error) {
	bundleIDEntitlemnstMap := archive.BundleIDEntitlementsMap()
	bundleIDs := []string{}
	for bundleID := range bundleIDEntitlemnstMap {
		bundleIDs = append(bundleIDs, bundleID)
	}

	codesignGroups := export.CreateSelectableCodeSignGroups(installedCertificates, installedProfiles, bundleIDs)
	if len(codesignGroups) == 0 {
		log.Errorf("Failed to find code singing groups for the project")
	}

	codesignGroups = export.FilterSelectableCodeSignGroups(codesignGroups,
		export.CreateEntitlementsSelectableCodeSignGroupFilter(bundleIDEntitlemnstMap),
		export.CreateExportMethodSelectableCodeSignGroupFilter(exportMethod),
	)

	macCodeSignGroups := export.CreateMacCodeSignGroup(codesignGroups, installedInstallerCertificates, exportMethod)
	if len(macCodeSignGroups) == 0 {
		return nil, fmt.Errorf("Can not create macos codesiging groups for the project")
	} else if len(macCodeSignGroups) > 1 {
		log.Warnf("Multiple matching  codesiging groups found for the project, using first...")
	}
	return &(macCodeSignGroups[0]), nil
}

func main() {
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}

	stepconf.Print(cfg)
	fmt.Println()
	log.SetEnableDebugLog(cfg.VerboseLog == "yes")

	log.Infof("step determined cfg:")

	// Detect Xcode major version
	xcodebuildVersion, err := utility.GetXcodeVersion()
	if err != nil {
		failf("Failed to get the version of xcodebuild! Error: %s", err)
	}
	log.Printf("- xcodebuild_version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)

	outputTool := cfg.OutputTool
	if outputTool == "xcpretty" {
		fmt.Println()
		log.Infof("Checking if output tool (xcpretty) is installed")

		installed, err := xcpretty.IsInstalled()
		if err != nil {
			log.Warnf("Failed to check if xcpretty is installed, error: %s", err)
			log.Printf("Switching to xcodebuild for output tool")
			outputTool = "xcodebuild"
		} else if !installed {
			log.Warnf("xcpretty is not installed")
			fmt.Println()
			log.Printf("Installing xcpretty")

			if cmds, err := xcpretty.Install(); err != nil {
				log.Warnf("Failed to create xcpretty install command: %s", err)
				log.Warnf("Switching to xcodebuild for output tool")
				outputTool = "xcodebuild"
			} else {
				for _, cmd := range cmds {
					if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
						if errorutil.IsExitStatusError(err) {
							log.Warnf("%s failed: %s", out)
						} else {
							log.Warnf("%s failed: %s", err)
						}
						log.Warnf("Switching to xcodebuild for output tool")
						outputTool = "xcodebuild"
					}
				}
			}
		}
	}

	if outputTool == "xcpretty" {
		xcprettyVersion, err := xcpretty.Version()
		if err != nil {
			log.Warnf("Failed to determine xcpretty version, error: %s", err)
			log.Printf("Switching to xcodebuild for output tool")
			outputTool = "xcodebuild"
		}
		log.Printf("- xcprettyVersion: %s", xcprettyVersion.String())
	}

	// Validation CustomExportOptionsPlistContent
	if cfg.CustomExportOptionsPlistContent != "" &&
		xcodebuildVersion.MajorVersion < 7 {
		log.Warnf("CustomExportOptionsPlistContent is set, but CustomExportOptionsPlistContent only used if xcodeMajorVersion > 6")
		cfg.CustomExportOptionsPlistContent = ""
	}

	if cfg.ForceProvisioningProfileSpecifier != "" &&
		xcodebuildVersion.MajorVersion < 8 {
		log.Warnf("ForceProvisioningProfileSpecifier is set, but ForceProvisioningProfileSpecifier only used if xcodeMajorVersion > 7")
		cfg.ForceProvisioningProfileSpecifier = ""
	}

	if cfg.ForceTeamID == "" &&
		xcodebuildVersion.MajorVersion < 8 {
		log.Warnf("ForceTeamID is set, but ForceTeamID only used if xcodeMajorVersion > 7")
		cfg.ForceTeamID = ""
	}

	if cfg.ForceProvisioningProfileSpecifier != "" &&
		cfg.ForceProvisioningProfile != "" {
		log.Warnf("both ForceProvisioningProfileSpecifier and ForceProvisioningProfile are set, using ForceProvisioningProfileSpecifier")
		cfg.ForceProvisioningProfile = ""
	}

	// Project-or-Workspace flag
	action := ""
	if strings.HasSuffix(cfg.ProjectPath, ".xcodeproj") {
		action = "-project"
	} else if strings.HasSuffix(cfg.ProjectPath, ".xcworkspace") {
		action = "-workspace"
	} else {
		failf("Invalid project file (%s), extension should be (.xcodeproj/.xcworkspace)", cfg.ProjectPath)
	}

	log.Printf("- action: %s", action)

	// export format
	exportFormat := "app"
	if cfg.ExportMethod == "app-store" {
		exportFormat = "pkg"
	}
	log.Printf("- export_format: %s", exportFormat)

	fmt.Println()

	// abs out dir pth
	absOutputDir, err := pathutil.AbsPath(cfg.OutputDir)
	if err != nil {
		failf("Failed to expand OutputDir (%s), error: %s", cfg.OutputDir, err)
	}
	cfg.OutputDir = absOutputDir

	if exist, err := pathutil.IsPathExists(cfg.OutputDir); err != nil {
		failf("Failed to check if OutputDir exist, error: %s", err)
	} else if !exist {
		if err := os.MkdirAll(cfg.OutputDir, 0777); err != nil {
			failf("Failed to create OutputDir (%s), error: %s", cfg.OutputDir, err)
		}
	}

	// output files
	archiveTempDir, err := pathutil.NormalizedOSTempDirPath("bitrise-xcarchive")
	if err != nil {
		failf("Failed to create archive tmp dir, error: %s", err)
	}

	archivePath := filepath.Join(archiveTempDir, cfg.ArtifactName+".xcarchive")
	log.Printf("- archivePath: %s", archivePath)

	archiveZipPath := filepath.Join(cfg.OutputDir, cfg.ArtifactName+".xcarchive.zip")
	log.Printf("- archiveZipPath: %s", archiveZipPath)

	exportOptionsPath := filepath.Join(cfg.OutputDir, "export_options.plist")
	log.Printf("- exportOptionsPath: %s", exportOptionsPath)

	filePath := filepath.Join(cfg.OutputDir, cfg.ArtifactName+"."+exportFormat)
	log.Printf("- filePath: %s", filePath)

	dsymZipPath := filepath.Join(cfg.OutputDir, cfg.ArtifactName+".dSYM.zip")
	log.Printf("- dsymZipPath: %s", dsymZipPath)

	rawXcodebuildOutputLogPath := filepath.Join(cfg.OutputDir, "raw-xcodebuild-output.log")
	log.Printf("- rawXcodebuildOutputLogPath: %s", rawXcodebuildOutputLogPath)

	ideDistributionLogsZipPath := filepath.Join(cfg.OutputDir, "xcodebuild.xcdistributionlogs.zip")
	log.Printf("- ideDistributionLogsZipPath: %s", ideDistributionLogsZipPath)

	fmt.Println()

	// clean-up
	filesToCleanup := []string{
		filePath,
		dsymZipPath,
		rawXcodebuildOutputLogPath,
		archiveZipPath,
		exportOptionsPath,
	}

	for _, pth := range filesToCleanup {
		if exist, err := pathutil.IsPathExists(pth); err != nil {
			failf("Failed to check if path (%s) exist, error: %s", pth, err)
		} else if exist {
			if err := os.RemoveAll(pth); err != nil {
				failf("Failed to remove path (%s), error: %s", pth, err)
			}
		}
	}

	//
	// Create the Archive with Xcode Command Line tools
	log.Infof("Create archive ...")
	fmt.Println()

	isWorkspace := false
	ext := filepath.Ext(cfg.ProjectPath)
	if ext == ".xcodeproj" {
		isWorkspace = false
	} else if ext == ".xcworkspace" {
		isWorkspace = true
	} else {
		failf("Project file extension should be .xcodeproj or .xcworkspace, but got: %s", ext)
	}

	archiveCmd := xcodebuild.NewCommandBuilder(cfg.ProjectPath, isWorkspace, xcodebuild.ArchiveAction)
	archiveCmd.SetScheme(cfg.Scheme)
	archiveCmd.SetConfiguration(cfg.Configuration)

	if cfg.ForceTeamID != "" {
		log.Printf("Forcing Development Team: %s", cfg.ForceTeamID)
		archiveCmd.SetForceDevelopmentTeam(cfg.ForceTeamID)
	}
	if cfg.ForceProvisioningProfileSpecifier != "" {
		log.Printf("Forcing Provisioning Profile Specifier: %s", cfg.ForceProvisioningProfileSpecifier)
		archiveCmd.SetForceProvisioningProfileSpecifier(cfg.ForceProvisioningProfileSpecifier)
	}
	if cfg.ForceProvisioningProfile != "" {
		log.Printf("Forcing Provisioning Profile: %s", cfg.ForceProvisioningProfile)
		archiveCmd.SetForceProvisioningProfile(cfg.ForceProvisioningProfile)
	}
	if cfg.ForceCodeSignIdentity != "" {
		log.Printf("Forcing Code Signing Identity: %s", cfg.ForceCodeSignIdentity)
		archiveCmd.SetForceCodeSignIdentity(cfg.ForceCodeSignIdentity)
	}

	if cfg.IsCleanBuild == "yes" {
		archiveCmd.SetCustomBuildAction("clean")
	}

	archiveCmd.SetDisableIndexWhileBuilding(cfg.DisableIndexWhileBuilding)
	archiveCmd.SetArchivePath(archivePath)

	if outputTool == "xcpretty" {
		xcprettyCmd := xcpretty.New(archiveCmd)

		log.TSuccessf("$ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		if rawXcodebuildOut, err := xcprettyCmd.Run(); err != nil {

			log.Errorf("\nLast lines of the Xcode's build log:")
			fmt.Println(stringutil.LastNLines(rawXcodebuildOut, 10))

			if err := output.ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
				log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
			} else {
				log.Warnf(`You can find the last couple of lines of Xcode's build log above, but the full log is also available in the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable
(value: %s)`, rawXcodebuildOutputLogPath)
			}

			failf("Archive failed, error: %s", err)
		}
	} else {
		log.TSuccessf("$ %s", archiveCmd.PrintableCmd())
		fmt.Println()

		if err := archiveCmd.Run(); err != nil {
			failf("Archive failed, error: %s", err)
		}
	}

	// Ensure xcarchive exists
	if exist, err := pathutil.IsPathExists(archivePath); err != nil {
		failf("Failed to check if archive exist, error: %s", err)
	} else if !exist {
		failf("No archive generated at: %s", archivePath)
	}

	archive, err := xcarchive.NewMacosArchive(archivePath)
	if err != nil {
		failf("Failed to parse archive, error: %s", err)
	}

	identity := archive.SigningIdentity()

	log.Infof("Archive infos:")
	log.Printf("codesign identity: %v", identity)
	fmt.Println()

	// Exporting xcarchive
	fmt.Println()
	log.Infof("Exporting xcarchive ...")
	fmt.Println()

	if err := output.ExportOutputDir(archivePath, archivePath, bitriseXCArchiveDirPthEnvKey); err != nil {
		failf("Failed to export %s, error: %s", bitriseXCArchiveDirPthEnvKey, err)
	}

	log.Donef("The xcarchive path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchiveDirPthEnvKey, archivePath)

	if cfg.IsExportXcarchiveZip == "yes" {
		if err := output.ZipAndExportOutput(archivePath, archiveZipPath, bitriseXCArchivePthEnvKey); err != nil {
			failf("Failed to export %s, error: %s", bitriseXCArchivePthEnvKey, err)
		}

		log.Donef("The xcarchive zip path is now available in the Environment Variable: %s (value: %s)", bitriseXCArchivePthEnvKey, archiveZipPath)
	}

	fmt.Println()

	// Export APP from generated archive
	log.Infof("Exporting APP from generated Archive ...")

	envsToUnset := []string{"GEM_HOME", "GEM_PATH", "RUBYLIB", "RUBYOPT", "BUNDLE_BIN_PATH", "_ORIGINAL_GEM_PATH", "BUNDLE_GEMFILE"}
	for _, key := range envsToUnset {
		if err := os.Unsetenv(key); err != nil {
			failf("Failed to unset (%s), error: %s", key, err)
		}
	}

	// Legacy
	if cfg.ExportMethod == "none" {
		log.Printf("Export a copy of the application without re-signing...")
		fmt.Println()

		embeddedAppPattern := filepath.Join(archivePath, "Products", "Applications", "*.app")
		matches, err := filepath.Glob(embeddedAppPattern)
		if err != nil {
			failf("Failed to find embedded app with pattern: %s, error: %s", embeddedAppPattern, err)
		}

		if len(matches) == 0 {
			failf("No embedded app found with pattern: %s", embeddedAppPattern)
		} else if len(matches) > 1 {
			failf("Multiple embedded app found with pattern: %s", embeddedAppPattern)
		}

		embeddedAppPath := matches[0]
		appPath := filepath.Join(cfg.OutputDir, cfg.ArtifactName+".app")

		if err := output.ExportOutputDir(embeddedAppPath, appPath, bitriseAppPthEnvKey); err != nil {
			failf("Failed to export %s, error: %s", bitriseAppPthEnvKey, err)
		}

		log.Donef("The app path is now available in the Environment Variable: %s (value: %s)", bitriseAppPthEnvKey, appPath)

		filePath = filePath + ".zip"
		if err := output.ZipAndExportOutput(embeddedAppPath, filePath, bitriseExportedFilePath); err != nil {
			failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
		}

		log.Donef("The app.zip path is now available in the Environment Variable: %s (value: %s)", bitriseExportedFilePath, filePath)
	} else {
		// export using exportOptions
		log.Printf("Export using exportOptions...")
		fmt.Println()

		exportTmpDir, err := pathutil.NormalizedOSTempDirPath("__export__")
		if err != nil {
			failf("Failed to create export tmp dir, error: %s", err)
		}

		exportCmd := xcodebuild.NewExportCommand()
		exportCmd.SetArchivePath(archivePath)
		exportCmd.SetExportDir(exportTmpDir)

		if cfg.CustomExportOptionsPlistContent != "" {
			log.Printf("Custom export options content provided:")
			fmt.Println(cfg.CustomExportOptionsPlistContent)

			if err := fileutil.WriteStringToFile(exportOptionsPath, cfg.CustomExportOptionsPlistContent); err != nil {
				failf("Failed to write export options to file, error: %s", err)
			}
		} else {
			exportMethod, err := exportoptions.ParseMethod(cfg.ExportMethod)
			if err != nil {
				failf("Failed to parse export method, error: %s", err)
			}

			var macCSGroup *export.MacCodeSignGroup
			exportProfileMapping := map[string]string{}

			// We do not need provisioning profile for the export if the app in the generated XcArchive doesn't
			// contain embedded provisioning profile.
			// The only exception is the DeveloperID export. For DeveloperID export we always have to have
			// provisioning profile.
			if archive.Application.ProvisioningProfile != nil || exportMethod == exportoptions.MethodDeveloperID {
				installedCertificates, err := certificateutil.InstalledCodesigningCertificateInfos()
				if err != nil {
					failf("Failed to get installed certificates, error: %s", err)
				}
				installedCertificates = certificateutil.FilterValidCertificateInfos(installedCertificates)

				log.Debugf("\n")
				log.Debugf("Installed certificates:")
				for _, certInfo := range installedCertificates {
					log.Debugf(certInfo.String())
				}

				installedProfiles, err := profileutil.InstalledProvisioningProfileInfos(profileutil.ProfileTypeMacOs)
				if err != nil {
					failf("Failed to get installed provisioning profiles, error: %s", err)
				}

				log.Debugf("\n")
				log.Debugf("Installed profiles:")
				for _, profInfo := range installedProfiles {
					log.Debugf(profInfo.String())
				}

				var installedInstallerCertificates []certificateutil.CertificateInfoModel
				if exportMethod == exportoptions.MethodAppStore {
					installedInstallerCertificates, err = certificateutil.InstalledInstallerCertificateInfos()
					if err != nil {
						log.Errorf("Failed to read installed Installer certificates, error: %s", err)
					}
					installedInstallerCertificates = certificateutil.FilterValidCertificateInfos(installedInstallerCertificates)

					log.Debugf("\n")
					log.Debugf("Installed installer certificates:")
					for _, certInfo := range installedInstallerCertificates {
						log.Debugf(certInfo.String())
					}
				}

				codesignGroup, err := macCodeSignGroup(archive, installedCertificates, installedInstallerCertificates, installedProfiles, exportMethod)
				if err != nil {
					failf("Failed to find code sign groups for the project, error: %s", err)
				}

				if codesignGroup != nil {
					for bundleID, profileInfo := range codesignGroup.BundleIDProfileMap() {
						exportProfileMapping[bundleID] = profileInfo.Name
					}
				}
			} else {
				log.Warnf("Archive was generated without provisioning profile and the export method is not DeveloperID")
				log.Printf("Export the application without re-signing...")
				fmt.Println()
			}

			var exportOpts exportoptions.ExportOptions
			if exportMethod == exportoptions.MethodAppStore {
				options := exportoptions.NewAppStoreOptions()

				if macCSGroup != nil {
					options.BundleIDProvisioningProfileMapping = exportProfileMapping
					options.SigningCertificate = macCSGroup.Certificate().CommonName
					options.InstallerSigningCertificate = macCSGroup.InstallerCertificate().CommonName
				}

				exportOpts = options
			} else {
				options := exportoptions.NewNonAppStoreOptions(exportMethod)

				if macCSGroup != nil {
					options.BundleIDProvisioningProfileMapping = exportProfileMapping
					options.SigningCertificate = macCSGroup.Certificate().CommonName
				}

				exportOpts = options
			}

			log.Printf("generated export options content:")
			fmt.Println()
			fmt.Println(exportOpts.String())

			if err = exportOpts.WriteToFile(exportOptionsPath); err != nil {
				failf("Failed to write export options to file, error: %s", err)
			}
		}

		exportCmd.SetExportOptionsPlist(exportOptionsPath)

		if outputTool == "xcpretty" {
			xcprettyCmd := xcpretty.New(exportCmd)

			log.Donef("$ %s", xcprettyCmd.PrintableCmd())
			fmt.Println()

			if xcodebuildOut, err := xcprettyCmd.Run(); err != nil {
				// xcodebuild raw output
				if err := output.ExportOutputFileContent(xcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
				} else {
					log.Warnf(`If you can't find the reason of the error in the log, please check the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable (value: %s)`, rawXcodebuildOutputLogPath)
				}

				// xcdistributionlogs
				if logsDirPth, err := findIDEDistrubutionLogsPath(xcodebuildOut); err != nil {
					log.Warnf("Failed to find xcdistributionlogs, error: %s", err)
				} else if err := output.ZipAndExportOutput(logsDirPth, ideDistributionLogsZipPath, bitriseIDEDistributionLogsPthEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseIDEDistributionLogsPthEnvKey, err)
				} else {
					criticalDistLogFilePth := filepath.Join(logsDirPth, "IDEDistribution.critical.log")
					log.Warnf("IDEDistribution.critical.log:")
					if criticalDistLog, err := fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
						log.Printf(criticalDistLog)
					}

					log.Warnf(`If you can't find the reason of the error in the log, please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable (value: %s)`, ideDistributionLogsZipPath)
				}

				failf("Export failed, error: %s", err)
			}
		} else {
			log.Donef("$ %s", exportCmd.PrintableCmd())
			fmt.Println()

			if xcodebuildOut, err := exportCmd.RunAndReturnOutput(); err != nil {
				// xcdistributionlogs
				if logsDirPth, err := findIDEDistrubutionLogsPath(xcodebuildOut); err != nil {
					log.Warnf("Failed to find xcdistributionlogs, error: %s", err)
				} else if err := output.ZipAndExportOutput(logsDirPth, ideDistributionLogsZipPath, bitriseIDEDistributionLogsPthEnvKey); err != nil {
					log.Warnf("Failed to export %s, error: %s", bitriseIDEDistributionLogsPthEnvKey, err)
				} else {
					criticalDistLogFilePth := filepath.Join(logsDirPth, "IDEDistribution.critical.log")
					log.Warnf("IDEDistribution.critical.log:")
					if criticalDistLog, err := fileutil.ReadStringFromFile(criticalDistLogFilePth); err == nil {
						log.Printf(criticalDistLog)
					}

					log.Warnf(`If you can't find the reason of the error in the log, please check the xcdistributionlogs
The logs directory is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_IDEDISTRIBUTION_LOGS_PATH environment variable (value: %s)`, ideDistributionLogsZipPath)
				}

				failf("Export failed, error: %s", err)
			}
		}

		// find exported app
		pattern := filepath.Join(exportTmpDir, "*."+exportFormat)
		apps, err := filepath.Glob(pattern)
		if err != nil {
			failf("Failed to find app, with pattern: %s, error: %s", pattern, err)
		}

		if len(apps) > 0 {
			if exportFormat == "pkg" {
				if err := output.ExportOutputFile(apps[0], filePath, bitriseExportedFilePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
				}
			} else {
				if err := tools.ExportEnvironmentWithEnvman(bitriseAppPthEnvKey, filePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseAppPthEnvKey, err)
				}
				filePath = filePath + ".zip"
				if err := output.ZipAndExportOutput(apps[0], filePath, bitriseExportedFilePath); err != nil {
					failf("Failed to export %s, error: %s", bitriseExportedFilePath, err)
				}
			}

			fmt.Println()
			log.Donef("The app path is now available in the Environment Variable: %s (value: %s)", bitriseExportedFilePath, filePath)
		}
	}

	// Export .dSYM files
	fmt.Println()
	log.Infof("Exporting dSYM files ...")
	fmt.Println()

	appDSYM, frameworkDSYMs, err := archive.FindDSYMs()
	if err != nil {
		failf("Failed to export dsyms, error: %s", err)
	}

	dsymDir, err := pathutil.NormalizedOSTempDirPath("__dsyms__")
	if err != nil {
		failf("Failed to create tmp dir, error: %s", err)
	}

	if err := command.CopyDir(appDSYM, dsymDir, false); err != nil {
		failf("Failed to copy (%s) -> (%s), error: %s", appDSYM, dsymDir, err)
	}

	if cfg.IsExportAllDsyms == "yes" {
		for _, dsym := range frameworkDSYMs {
			if err := command.CopyDir(dsym, dsymDir, false); err != nil {
				failf("Failed to copy (%s) -> (%s), error: %s", dsym, dsymDir, err)
			}
		}
	}

	if err := output.ZipAndExportOutput(dsymDir, dsymZipPath, bitriseDSYMDirPthEnvKey); err != nil {
		failf("Failed to export %s, error: %s", bitriseDSYMDirPthEnvKey, err)
	}

	log.Donef("The dSYM dir path is now available in the Environment Variable: %s (value: %s)", bitriseDSYMDirPthEnvKey, dsymZipPath)
}
