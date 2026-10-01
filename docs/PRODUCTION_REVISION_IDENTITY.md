# Production Revision Identity

Production identity is verified as a chain, not inferred from service health:

`Git HEAD -> stamped build commit -> /version -> build-manifest.json -> SHA256 of running API/worker artifacts -> active systemd services`

Run `python3 tools/verify-production-revision.py`. A successful result requires every link in that chain to match. The machine-readable latest verification is `docs/PRODUCTION_REVISION_STATUS.json`.

`modified` in `/version` is Go VCS metadata and may be true when unrelated working-tree audit artifacts existed at build time; the deployed source revision is still verified independently through the stamped commit and artifact hashes.
