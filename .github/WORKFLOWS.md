# GitHub Actions Workflows

This directory contains all GitHub Actions workflows for the DTVN project. These workflows automate testing, building, security scanning, and release processes.

## 📋 Available Workflows

### 1. **Go CI** (`go-ci.yml`)

**Triggers**: Push/PR to `main` or `develop` branches

**Purpose**: Continuous Integration for Go code

**Jobs**:
- **Build and Test**: Builds the project and runs tests on Go 1.24.x and 1.25.x
- **Lint**: Runs golangci-lint to check code quality
- **Format Check**: Ensures code is properly formatted with `gofmt`
- **Protocol Buffers Check**: Verifies proto files are up to date

**Status Badge**:
```markdown
[![Go CI](https://github.com/Hetti219/DTVN/actions/workflows/go-ci.yml/badge.svg)](https://github.com/Hetti219/DTVN/actions/workflows/go-ci.yml)
```

---

### 2. **Docker Build and Publish** (`docker.yml`)

**Triggers**:
- Push to `main` or `develop` branches
- Push of tags matching `v*`
- Pull requests to `main`

**Purpose**: Build Docker images and publish to GitHub Container Registry

**Jobs**:
- **Build**: Builds multi-platform Docker images (amd64, arm64)
- **Security Scan**: Runs Trivy vulnerability scanner
- **Test**: Tests Docker image functionality (PR only)

**Images Published**: `ghcr.io/hetti219/dtvn:latest`, `ghcr.io/hetti219/dtvn:<tag>`

**Status Badge**:
```markdown
[![Docker](https://github.com/Hetti219/DTVN/actions/workflows/docker.yml/badge.svg)](https://github.com/Hetti219/DTVN/actions/workflows/docker.yml)
```

---

### 3. **Security Scanning** (`security.yml`)

**Triggers**:
- Push/PR to `main` or `develop` branches
- Weekly schedule (Mondays at midnight)

**Purpose**: Comprehensive security analysis

**Jobs**:
- **CodeQL Analysis**: Static analysis for security vulnerabilities
- **Dependency Review**: Reviews dependency changes in PRs
- **Gosec**: Go-specific security checker
- **Govulncheck**: Checks for known vulnerabilities in dependencies

**Status Badge**:
```markdown
[![Security](https://github.com/Hetti219/DTVN/actions/workflows/security.yml/badge.svg)](https://github.com/Hetti219/DTVN/actions/workflows/security.yml)
```

---

### 4. **Release** (`release.yml`)

**Triggers**: Push of tags matching `v*`

**Purpose**: Automated release process with GoReleaser

**Jobs**:
- **GoReleaser**: Builds binaries for multiple platforms
  - Generates SBOMs (Software Bill of Materials)
  - Signs binaries with Cosign
  - Creates GitHub release with artifacts
- **Docker Release**: Builds and publishes release Docker images
- **Release Notes**: Generates changelog and updates release

**How to Create a Release**:
```bash
# Create and push a tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

**Status Badge**:
```markdown
[![Release](https://github.com/Hetti219/DTVN/actions/workflows/release.yml/badge.svg)](https://github.com/Hetti219/DTVN/actions/workflows/release.yml)
```

---

### 5. **Pull Request Checks** (`pr-checks.yml`)

**Triggers**: PR opened, synchronized, or reopened

**Purpose**: PR quality and metadata checks

**Jobs**:
- **PR Metadata Check**: Ensures PR titles follow conventional commits
- **PR Size Check**: Labels PRs by size and warns about large PRs
- **Conflict Check**: Detects merge conflicts
- **Auto Assign**: Automatically assigns reviewers

**PR Title Format**:
```
<type>: <description>

Examples:
feat: Add ticket validation API
fix: Resolve consensus timeout issue
docs: Update README with installation steps
```

---

## 🔒 Security Features

### Dependabot (`dependabot.yml`)

Automated dependency updates for:
- **Go modules**: Weekly updates on Mondays
- **GitHub Actions**: Weekly updates
- **Docker base images**: Weekly updates

**Configuration**:
- Ignores major version updates by default (to prevent breaking changes)
- Automatically assigns PRs to @Hetti219
- Labels PRs by category (go, github-actions, docker)

### Security Scanning

- **CodeQL**: Advanced semantic code analysis
- **Gosec**: Go security checker
- **Govulncheck**: Go vulnerability database check
- **Trivy**: Container vulnerability scanner
- **Dependency Review**: Blocks PRs with vulnerable dependencies

---

## 🚀 Getting Started

### Prerequisites

1. **Enable GitHub Actions** in your repository settings
2. **Enable GitHub Container Registry**:
   - Go to Settings → Packages
   - Enable package creation

3. **Configure Secrets** (if needed):
   - `GITHUB_TOKEN` is automatically provided
   - No additional secrets required for basic functionality

### First Run

After pushing these workflows:

1. **Go CI** will run immediately on push
2. **Security Scanning** will run and may take 5-10 minutes
3. **Docker Build** will publish images to GHCR

### Using Docker Images

Pull the latest image:
```bash
docker pull ghcr.io/hetti219/dtvn:latest
```

Pull a specific version:
```bash
docker pull ghcr.io/hetti219/dtvn:v1.0.0
```

---

## 📊 Monitoring Workflows

### View Workflow Status

1. Go to the **Actions** tab in your GitHub repository
2. Select a workflow from the left sidebar
3. Click on a specific run to see details

### Debugging Failed Workflows

1. Click on the failed job
2. Expand the failed step
3. Review logs for error messages
4. Common issues:
   - **Test failures**: Check test output in "Run tests" step
   - **Lint errors**: Check golangci-lint output
   - **Docker build failures**: Check Dockerfile and dependencies
   - **Permission errors**: Ensure GITHUB_TOKEN has correct permissions

---

## 🔧 Customization

### Modify Workflow Triggers

Edit the `on:` section in workflow files:

```yaml
on:
  push:
    branches: [ main, develop, feature/* ]
  pull_request:
    branches: [ main ]
```

### Add More Go Versions

Edit `go-ci.yml`:

```yaml
strategy:
  matrix:
    go-version: ['1.23.x', '1.24.x', '1.25.x']
```

### Change Release Configuration

Edit `.goreleaser.yaml` to customize:
- Build targets (OS/architecture)
- Archive formats
- Release notes format
- SBOM generation

---

## 📈 Best Practices

1. **Keep workflows fast**: Use caching for dependencies
2. **Run tests locally first**: Use `make test` before pushing
3. **Review Dependabot PRs**: Don't auto-merge without testing
4. **Monitor security alerts**: Check Security tab regularly
5. **Use conventional commits**: Helps with automated changelogs
6. **Keep PRs small**: Easier to review and test

---

## 🆘 Troubleshooting

### Workflow Not Running

- Check if Actions are enabled in repository settings
- Verify workflow file syntax with GitHub's validator
- Check branch protection rules

### Docker Push Failed

- Verify GITHUB_TOKEN has `packages:write` permission
- Check if container registry is enabled
- Ensure you're authenticated to GHCR

### Release Failed

- Ensure tag follows `v*` pattern (e.g., `v1.0.0`)
- Check `.goreleaser.yaml` syntax
- Verify all required tools are available (Cosign, Syft)

### Security Scan False Positives

- Review the specific vulnerability
- Add exceptions in workflow if needed
- Update dependencies to patched versions

---

## 📚 Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [GoReleaser Documentation](https://goreleaser.com/)
- [Dependabot Configuration](https://docs.github.com/en/code-security/dependabot)
- [CodeQL for Go](https://codeql.github.com/docs/codeql-language-guides/codeql-for-go/)
- [Docker Build Push Action](https://github.com/docker/build-push-action)

---

## 📝 Workflow Summary

| Workflow | Frequency | Purpose | Blocking |
|----------|-----------|---------|----------|
| Go CI | Every push/PR | Build, test, lint | ✅ Yes |
| Docker | Push to main/tags | Build images | ❌ No |
| Security | Weekly + Push/PR | Security scans | ⚠️ Optional |
| Release | On tag push | Create releases | ✅ Yes |
| PR Checks | On PR | PR quality | ❌ No |

---

**Last Updated**: 2025-12-04
