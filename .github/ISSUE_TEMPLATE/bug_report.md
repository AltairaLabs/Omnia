---
name: Bug report
about: Create a report to help us improve
title: '[BUG] '
labels: ['bug', 'needs-triage']
assignees: ''
---

## Bug Description

**Brief Summary**
A clear and concise description of what the bug is.

**Component Affected**
- [ ] Operator
- [ ] AgentRuntime CRD
- [ ] PromptPack CRD
- [ ] ToolRegistry CRD
- [ ] WebSocket Facade
- [ ] Session Store
- [ ] Helm Chart
- [ ] Documentation
- [ ] Other: ___________

## Steps to Reproduce

1. Go to '...'
2. Click on '....'
3. Scroll down to '....'
4. See error

**Expected Behavior**
A clear and concise description of what you expected to happen.

**Actual Behavior**
A clear and concise description of what actually happened.

## Environment

**Omnia Version:** (e.g., v0.1.0, main branch commit hash)

**Kubernetes Version:** (e.g., 1.28.0)

**Kubernetes Distribution:**
- [ ] kind
- [ ] minikube
- [ ] EKS
- [ ] GKE
- [ ] AKS
- [ ] Other: ___________

**Operating System:** (for local development issues)
- [ ] macOS
- [ ] Linux
- [ ] Windows

**Go Version:** (e.g., 1.21.5)

## Configuration

**CRD Configuration:** (if applicable)
```yaml
# Paste your AgentRuntime, PromptPack, or ToolRegistry YAML here
```

**Helm Values:** (if applicable)
```yaml
# Paste relevant Helm values here
```

## Error Output

**Operator Logs:**
```
Paste operator logs here (kubectl logs -n omnia-system deployment/omnia-operator)
```

**Agent Pod Logs:** (if applicable)
```
Paste agent pod logs here
```

**Kubernetes Events:**
```
Paste relevant events here (kubectl get events -n <namespace>)
```

## Additional Context

**Screenshots**
If applicable, add screenshots to help explain your problem.

**Additional Information**
Add any other context about the problem here, such as:
- Does this happen consistently or intermittently?
- Did this work in a previous version?
- Are there any workarounds you've found?

## Checklist

- [ ] I have searched existing issues to ensure this is not a duplicate
- [ ] I have provided all the information requested above
- [ ] I have tested this with the latest version of Omnia
- [ ] I have included relevant configuration and error output
