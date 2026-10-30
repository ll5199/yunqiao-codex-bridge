# Yunqiao Codex Bridge — maintenance notes

## Keep the website tutorial in sync with the UI
Whenever a change alters what the client UI looks like (`frontend/src`), refresh the
website media in the same piece of work, without waiting to be asked:

1. `(cd frontend && npm run build) && node scripts/render-site-media.cjs`
   regenerates `docs/website/media/` (tutorial screenshots, the walkthrough video
   `yunqiao-tutorial.mp4` and its poster) from the real UI with a mocked backend.
   Update the captions/steps in that script when the flow changes.
2. Publish the new media on https://yunqiao.xyz/ (live files under
   `/www/wwwroot/yunqiao.xyz/`): the homepage hero video, and the tutorial images,
   which are embedded as base64 and matched by their `alt` text. Update tutorial
   wording for anything the UI no longer shows or newly shows.
3. Back up every live file first (`<file>.before-<change>-<UTC timestamp>`), verify
   from the public URLs after, and report the backups for rollback.

The live site is not generated from `docs/website/*.template.html`; those
templates are older than the live pages. Edit the live pages in place.

## Releases
- Bump `appVersion` in `config.go`, `versioninfo.json` (both version blocks and
  strings) and add `RELEASE_NOTES_v<version>.md`; merging to `main` publishes the
  GitHub release.
- Online updates read `https://index.velyn65.com/download/update-config.json`, which
  lists the update manifests (`latest.json`). Deploy order: back up, upload the exe
  and verify its SHA-256 from the public URL, update the fixed entry
  `YunqiaoCodexBridge.exe`, site download links, and `latest.json` last.
