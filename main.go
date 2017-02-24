package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"path/filepath"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-tools/go-xcode/utility"
	"github.com/bitrise-tools/go-xcode/xcodebuild"
	"github.com/bitrise-tools/go-xcode/xcpretty"
)

const (
	bitriseXcodeRawResultTextEnvKey = "BITRISE_XCODE_RAW_RESULT_TEXT_PATH"
)

// ConfigsModel ...
type ConfigsModel struct {
	ProjectPath              string
	Scheme                   string
	OutputTool               string
	IsCleanBuild             string
	Configuration            string
	OutputDir                string
	ForceCodeSignIdentity    string
	ForceProvisioningProfile string
	ExportOptionsPath        string
	ExportMethod             string
	IsExportXcarchiveZip     string
	IsExportAllDsyms         string
}

func (configs ConfigsModel) print() {
	fmt.Println()
	log.Infof("Configs:")
	log.Printf("- ProjectPath: %s", configs.ProjectPath)
	log.Printf("- Scheme: %s", configs.Scheme)
	log.Printf("- OutputTool: %s", configs.OutputTool)
	log.Printf("- IsCleanBuild: %s", configs.IsCleanBuild)
	log.Printf("- Configuration: %s", configs.Configuration)
	log.Printf("- OutputDir: %s", configs.OutputDir)
	log.Printf("- ForceCodeSignIdentity: %s", configs.ForceCodeSignIdentity)
	log.Printf("- ForceProvisioningProfile: %s", configs.ForceProvisioningProfile)
	log.Printf("- ExportOptionsPath: %s", configs.ExportOptionsPath)
	log.Printf("- ExportMethod: %s", configs.ExportMethod)
	log.Printf("- IsExportXcarchiveZip: %s", configs.IsExportXcarchiveZip)
	log.Printf("- IsExportAllDsyms: %s", configs.IsExportAllDsyms)
}

/*
# Validate parameters
echo_info "Configs:"
echo_details "* workdir: ${workdir}"
>echo_details "* project_path: ${project_path}"
>echo_details "* scheme: ${scheme}"
>echo_details "* configuration: ${configuration}"
>echo_details "* output_dir: ${output_dir}"
>echo_details "* force_code_sign_identity: ${force_code_sign_identity}"
>echo_details "* force_provisioning_profile: ${force_provisioning_profile}"
>echo_details "* export_options_path: ${export_options_path}"
>echo_details "* export_method: ${export_method}"
>echo_details "* is_clean_build: ${is_clean_build}"
>echo_details "* output_tool: ${output_tool}"
>echo_details "* is_export_xcarchive_zip: ${is_export_xcarchive_zip}"
>echo_details "* is_export_all_dsyms: $is_export_all_dsyms"
*/

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		ProjectPath:              os.Getenv("project_path"),
		Scheme:                   os.Getenv("scheme"),
		OutputTool:               os.Getenv("output_tool"),
		IsCleanBuild:             os.Getenv("is_clean_build"),
		Configuration:            os.Getenv("configuration"),
		OutputDir:                os.Getenv("output_dir"),
		ForceCodeSignIdentity:    os.Getenv("force_code_sign_identity"),
		ForceProvisioningProfile: os.Getenv("force_provisioning_profile"),
		ExportOptionsPath:        os.Getenv("export_options_path"),
		ExportMethod:             os.Getenv("export_method"),
		IsExportXcarchiveZip:     os.Getenv("is_export_xcarchive_zip"),
		IsExportAllDsyms:         os.Getenv("is_export_all_dsyms"),
	}
}

//--------------------
// Functions
//--------------------
func validateRequiredInput(value, key string) error {
	if value == "" {
		return fmt.Errorf("Missing required input: %s", key)
	}
	return nil
}

func validateRequiredInputWithOptions(value, key string, options []string) error {
	if err := validateRequiredInput(key, value); err != nil {
		return err
	}

	found := false
	for _, option := range options {
		if option == value {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("Invalid input: (%s) value: (%s), valid options: %s", key, value, strings.Join(options, ", "))
	}

	return nil
}

func (configs ConfigsModel) validate() error {
	// required
	if err := validateRequiredInput(configs.ProjectPath, "project_path"); err != nil {
		return err
	}
	if exists, err := pathutil.IsDirExists(configs.ProjectPath); err != nil {
		return err
	} else if !exists {
		return errors.New("ProjectPath directory does not exists: %s")
	}

	if err := validateRequiredInput(configs.OutputDir, "output_dir"); err != nil {
		return err
	}
	if exists, err := pathutil.IsDirExists(configs.OutputDir); err != nil {
		return err
	} else if !exists {
		return errors.New("OutputDir directory does not exists: %s")
	}

	if err := validateRequiredInput(configs.Scheme, "scheme"); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions(configs.OutputTool, "output_tool", []string{"xcpretty", "xcodebuild"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions(configs.IsCleanBuild, "is_clean_build", []string{"yes", "no"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions(configs.IsExportXcarchiveZip, "is_export_xcarchive_zip", []string{"yes", "no"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions(configs.IsExportAllDsyms, "is_export_all_dsyms", []string{"yes", "no"}); err != nil {
		return err
	}

	if err := validateRequiredInputWithOptions(configs.ExportMethod, "export_method", []string{"none", "app-store", "development", "developer-id"}); err != nil {
		return err
	}

	if os.Getenv("is_force_code_sign") != "no" {
		fmt.Println()
		log.Warnf("is_force_code_sign is deprecated!")
		log.Warnf("Use `force_code_sign_identity` and `force_provisioning_profile` instead.")
	}

	return nil
}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

// ExportEnvironmentWithEnvman ...
func ExportEnvironmentWithEnvman(keyStr, valueStr string) error {
	return command.New("envman", "add", "--key", keyStr).SetStdin(strings.NewReader(valueStr)).Run()
}

// GetXcprettyVersion ...
func GetXcprettyVersion() (string, error) {
	cmd := command.New("xcpretty", "-version")
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

// ExportOutputFile ...
func ExportOutputFile(sourcePth, destinationPth, envKey string) error {
	if sourcePth != destinationPth {
		if err := command.CopyFile(sourcePth, destinationPth); err != nil {
			return err
		}
	}

	return ExportEnvironmentWithEnvman(envKey, destinationPth)
}

// ExportOutputFileContent ...
func ExportOutputFileContent(content, destinationPth, envKey string) error {
	if err := fileutil.WriteStringToFile(destinationPth, content); err != nil {
		return err
	}

	return ExportOutputFile(destinationPth, destinationPth, envKey)
}

//--------------------
// Main
//--------------------

func main() {
	configs := createConfigsModelFromEnvs()
	configs.print()
	if err := configs.validate(); err != nil {
		failf("Issue with input: %s", err)
	}

	fmt.Println()
	log.Infof("Other Configs:")

	// Project-or-Workspace flag
	action := ""
	if strings.HasSuffix(configs.ProjectPath, ".xcodeproj") {
		action = "-project"
	} else if strings.HasSuffix(configs.ProjectPath, ".xcworkspace") {
		action = "-workspace"
	} else {
		failf("Invalid project file (%s), extension should be (.xcodeproj/.xcworkspace)", configs.ProjectPath)
	}

	log.Printf("- action: %s", action)

	// Output tools versions
	xcodebuildVersion, err := utility.GetXcodeVersion()
	if err != nil {
		failf("Failed to get the version of xcodebuild! Error: %s", err)
	}

	log.Printf("- xcodebuild_version: %s (%s)", xcodebuildVersion.Version, xcodebuildVersion.BuildVersion)
	if configs.ExportOptionsPath != "" && xcodebuildVersion.MajorVersion == 6 {
		log.Warnf("Xcode major version: 6, export_options_path only used if xcode major version > 6")
		configs.ExportOptionsPath = ""
	}

	// xcpretty version
	if configs.OutputTool == "xcpretty" {
		xcprettyVersion, err := GetXcprettyVersion()
		if err != nil {
			failf("Failed to get the xcpretty version! Error: %s", err)
		} else {
			log.Printf("- xcpretty_version: %s", xcprettyVersion)
		}
	}

	// export format
	exportFormat := "app"
	if configs.ExportMethod == "app-store" {
		exportFormat = "pkg"
	}
	log.Printf("- export_format: %s", exportFormat)

	// output files
	archiveTempDir, err := pathutil.NormalizedOSTempDirPath("bitrise-xcarchive")
	if err != nil {
		failf("Failed to create archive tmp dir, error: %s", err)
	}
	archivePath := filepath.Join(archiveTempDir, fmt.Sprintf("%s.xcarchive", configs.Scheme))
	log.Printf("- archivePath: %s", archivePath)

	filePath := fmt.Sprintf("%s/%s.%s", configs.OutputDir, configs.Scheme, exportFormat)
	log.Printf("- filePath: %s", filePath)

	dsymZipPath := fmt.Sprintf("%s/%s.dSYM.zip", configs.OutputDir, configs.Scheme)
	log.Printf("- dsymZipPath: %s", dsymZipPath)

	rawXcodebuildOutputLogPath := filepath.Join(configs.OutputDir, "raw-xcodebuild-output.log")

	fmt.Println()

	// clean-up
	if exists, err := pathutil.IsPathExists(filePath); err != nil {
		failf("Failed to check if path exists, error: %s", err)
	} else if exists {
		log.Warnf("App at path (%s) already exists - removing it", filePath)
		if err := os.RemoveAll(filePath); err != nil {
			failf("Failed to remove path: %s, error: %s", filePath, err)
		}
	}

	archiveCmd := xcodebuild.NewArchiveCommand(configs.ProjectPath, (action == "-workspace"))
	archiveCmd.SetScheme(configs.Scheme)
	archiveCmd.SetConfiguration(configs.Configuration)

	if configs.ForceProvisioningProfile != "" {
		log.Printf("Forcing Provisioning Profile: %s", configs.ForceProvisioningProfile)
		archiveCmd.SetForceProvisioningProfile(configs.ForceProvisioningProfile)
	}
	if configs.ForceCodeSignIdentity != "" {
		log.Printf("Forcing Code Signing Identity: %s", configs.ForceCodeSignIdentity)
		archiveCmd.SetForceCodeSignIdentity(configs.ForceCodeSignIdentity)
	}

	if configs.IsCleanBuild == "yes" {
		archiveCmd.SetCustomBuildAction("clean")
	}

	archiveCmd.SetArchivePath(archivePath)

	if configs.OutputTool == "xcpretty" {
		xcprettyCmd := xcpretty.New(archiveCmd)

		log.Donef("$ %s", xcprettyCmd.PrintableCmd())
		fmt.Println()

		if rawXcodebuildOut, err := xcprettyCmd.Run(); err != nil {
			if err := ExportOutputFileContent(rawXcodebuildOut, rawXcodebuildOutputLogPath, bitriseXcodeRawResultTextEnvKey); err != nil {
				log.Warnf("Failed to export %s, error: %s", bitriseXcodeRawResultTextEnvKey, err)
			} else {
				log.Warnf(`If you can't find the reason of the error in the log, please check the raw-xcodebuild-output.log
The log file is stored in $BITRISE_DEPLOY_DIR, and its full path
is available in the $BITRISE_XCODE_RAW_RESULT_TEXT_PATH environment variable`)
			}

			failf("Archive failed, error: %s", err)
		}
	} else {
		log.Donef("$ %s", archiveCmd.PrintableCmd())
		fmt.Println()

		if err := archiveCmd.Run(); err != nil {
			failf("Archive failed, error: %s", err)
		}
	}
}
