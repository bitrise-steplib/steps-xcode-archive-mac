# Xcode Archive for Mac

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-xcode-archive-mac?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-xcode-archive-mac/releases)

Create an archive for your macOS project so you can share it, upload it, deploy it and catch them
all! Well, maybe not the last one.

<details>
<summary>Description</summary>


</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

### Example

Create an archive and export the app, then deploy it as a build artifact:

```yaml
steps:
- certificate-and-profile-installer: {} # Requires certificates and profiles uploaded to Bitrise
- xcode-archive-mac:
    inputs:
    - scheme: $BITRISE_SCHEME
    - export_method: app-store
- deploy-to-bitrise-io: {}
```

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `export_method` | The method for exporting the application.  - `development`: Save a copy of the application signed with your Development identity. - `app-store`: Sign and package application for distribution in the Mac App Store. - `developer-id`: Save a copy of the application signed with your Developer ID. - `none`: Export a copy of the application without re-signing.  See `xcodebuild -help` for more information. | required | `development` |
| `custom_export_options_plist_content` | Used for Xcode version 7 and above.  Specifies a custom export options plist content that configures archive exporting. If empty, step generates these options based on provisioning profile, with default values.  Auto generated export options available for export methods:  - app-store - ad-hoc - enterprise - development  If the step doesn't find an export method based on the provisioning profile(s), the development method will be used.  Call `xcodebuild -help` for available export options. |  |  |
| `project_path` | A `.xcodeproj` or `.xcworkspace` path.  | required | `$BITRISE_PROJECT_PATH` |
| `scheme` | Scheme to use in archiving | required | `$BITRISE_SCHEME` |
| `configuration` | (optional) The configuration to use. By default, your Scheme defines which configuration (Debug, Release, ...) should be used, but you can overwrite it with this option. **Make sure that the Configuration you specify actually exists in your Xcode Project**. If it does not (for example, if you have a typo in the value of this input), Xcode will simply use the Configuration specified by the Scheme and will silently ignore this parameter!  |  |  |
| `is_clean_build` | Do a clean Xcode build before the archive? | required | `yes` |
| `workdir` | Working directory of the step. You can leave it empty to leave the working directory unchanged.  |  | `$BITRISE_SOURCE_DIR` |
| `xcodebuild_options` | Options added to the end of the xcodebuild call.  You can use multiple options, separated by a space character. Example: `-xcconfig PATH -verbose` |  |  |
| `disable_index_while_building` | Could make the build faster by adding `COMPILER_INDEX_STORE_ENABLE=NO` flag to the `xcodebuild` command which will disable the indexing during the build.  Indexing is needed for  * Autocomplete * Ability to quickly jump to definition * Get class and method help by alt clicking.  Which are not needed in CI environment.  **Note:** In Xcode you can turn off the `Index-WhileBuilding` feature  by disabling the `Enable Index-WhileBuilding Functionality` in the `Build Settings`.<br/> In CI environment you can disable it by adding `COMPILER_INDEX_STORE_ENABLE=NO` flag to the `xcodebuild` command. |  | `yes` |
| `force_team_id` | Used for Xcode version 8 and above.  Force xcodebuild to use the specified Developer Portal team during archive.  Format example:  - `1MZX23ABCD4` |  |  |
| `force_code_sign_identity` | Force xcodebuild to use specified Code Sign Identity.  Specify code signing identity as full ID (e.g. `Mac Developer: Bitrise Bot (VV2J4SV8V4)`) or specify code signing group ( `Mac Developer` or `Mac Distribution` ).  You also have to **specify the Identity in the format it's stored in Xcode project settings**, and **not how it's presented in the Xcode.app GUI**! **The input is case sensitive**: `Mac Distribution` works but `mac distribution` does not! |  |  |
| `force_provisioning_profile_specifier` | Used for Xcode version 8 and above.  Force xcodebuild to use specified Provisioning Profile.  How to get your Provisioning Profile Specifier:  - In Xcode make sure you disabled `Automatically manage signing` on your project's `General` tab - Now you can select your Provisioning Profile Specifier's name as `Provisioning Profile` input value on your project's `General` tab - `force_provisioning_profile_specifier` input value build up by the Team ID and the Provisioning Profile Specifier name, separated with slash character ('/'): `TEAM_ID/PROFILE_SPECIFIER_NAME`  Format example:  - `1MZX23ABCD4/My Provisioning Profile` |  |  |
| `force_provisioning_profile` | Force xcodebuild to use the specified Provisioning Profile.  Use Provisioning Profile's UUID. The profile's name is not accepted by xcodebuild.  How to get your UUID:  - In Xcode select your project -> Build Settings -> Code Signing - Select the desired Provisioning Profile, then scroll down in profile list and click on Other... - The popup will show your profile's UUID.  Format example:  - c5be4123-1234-4f9d-9843-0d9be985a068 |  |  |
| `output_tool` | If output_tool is set to xcpretty, the xcodebuild output will be prettified by xcpretty. If output_tool is set to xcodebuild, the raw xcodebuild output will be printed. | required | `xcpretty` |
| `output_dir` | This directory will contain the generated .app or .pkg file's and .dSYM.zip files.  |  | `$BITRISE_DEPLOY_DIR` |
| `artifact_name` | This name will be used as basename for the generated .xcarchive, .app or .pkg and .dSYM.zip files. | required | `${scheme}` |
| `is_export_xcarchive_zip` | If this input is set to `yes`, the generated .xcarchive will be zipped and moved to `output_dir`.  | required | `no` |
| `is_export_all_dsyms` | If this input is set to `yes` step will collect every dsym (.app dsym and framwork dsyms) in a directory, zip it and export the zipped directory path. Otherwise only .app dsym will be zipped and the zip path exported. | required | `no` |
| `verbose_log` | Enable verbose logging? | required | `no` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_EXPORTED_FILE_PATH` | The created .app.zip or .pkg file's path |
| `BITRISE_APP_PATH` | The created .app path |
| `BITRISE_DSYM_PATH` | The created .dSYM.zip file's path |
| `BITRISE_XCARCHIVE_PATH` | The created .xcarchive.zip file's path |
| `BITRISE_MACOS_XCARCHIVE_PATH` | The created .xcarchive dir's path |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-xcode-archive-mac/pulls) and [issues](https://github.com/bitrise-steplib/steps-xcode-archive-mac/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

**Note:** this step's end-to-end tests (defined in `e2e/bitrise.yml`) are working with secrets which are intentionally not stored in this repo. External contributors won't be able to run those tests. Don't worry, if you open a PR with your contribution, we will help with running tests and make sure that they pass.

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
