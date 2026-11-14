# Abandoned branch — do not merge

This branch is a leftover of the Wave 1 remediation work and **must not be merged**.

It is missing `.github/workflows/go-ci.yml`. The GitHub connector available to the automation
refuses to create or modify any file under `.github/workflows/` (deletes are permitted, content
changes are not), so the file could not be restored after an experiment deleted it.

The finished Wave 1 work is on branch **`fix/wave1-build-ci-v2`** (PR #79). Use that branch, or
apply `dtvn-wave1-workflow-go-version.patch` to it to add the two missing CI Go-version bumps.

To remove this branch: `git push origin --delete fix/wave1-build-ci`
