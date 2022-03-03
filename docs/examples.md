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