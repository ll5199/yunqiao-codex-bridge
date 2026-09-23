# Yunqiao Wails UI Preview

This Windows executable is an interface-only preview of the compact console concept. It does not authenticate, persist credentials, connect to services, synchronize models, update the client, or start Codex. Those controls show preview-only status messages.

The GitHub Actions workflow builds on a Windows runner and scans the resulting EXE with Microsoft Defender before uploading a short-lived artifact. If Defender reports a detection or scanning is unavailable, the artifact is withheld.
