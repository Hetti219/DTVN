## Description

<!-- Briefly describe what this PR does and why -->

## Type of Change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Refactoring (no functional changes)
- [ ] Documentation update
- [ ] CI/CD or infrastructure change

## Changes Made

<!-- List the key changes in bullet points -->

-

## How Has This Been Tested?

<!-- Describe the tests you ran and how to reproduce them -->

- [ ] Unit tests (`make test-unit`)
- [ ] Integration tests (`make test-integration`)
- [ ] Manual testing (describe below)

**Manual test steps:**

1.

## Consensus & Protocol Safety

<!-- Only applicable if modifying pkg/consensus, pkg/gossip, or pkg/state -->

- [ ] N/A - No protocol changes
- [ ] Byzantine fault tolerance properties maintained
- [ ] Quorum size remains 2f+1
- [ ] Tested with simulator under network failures

## Checklist

- [ ] My code follows the existing code style of this project
- [ ] I have performed a self-review of my own code
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally (`make test`)
- [ ] Build succeeds (`make build`)
