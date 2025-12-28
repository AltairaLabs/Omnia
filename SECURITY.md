# Security Policy

## Supported Versions

We take security seriously and provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| Latest release | :white_check_mark: |
| Previous release | :white_check_mark: |
| < Previous release | :x: |

## Reporting a Vulnerability

We appreciate responsible disclosure of security vulnerabilities. If you discover a security issue, please follow these steps:

### 1. Do Not Create Public Issues

**Please do not report security vulnerabilities through public GitHub issues.** Public disclosure before a fix is available can put users at risk.

### 2. Report Privately

Send an email to our security team at: **[security@altairalabs.ai](mailto:security@altairalabs.ai)**

Include the following information in your report:
- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact and attack scenarios
- Any suggested fixes or mitigations
- Your contact information for follow-up

### 3. Encryption (Optional)

For highly sensitive reports, you may encrypt your email using our PGP key:

```
-----BEGIN PGP PUBLIC KEY BLOCK-----
[PGP key will be provided upon request]
-----END PGP PUBLIC KEY BLOCK-----
```

### 4. Response Timeline

We are committed to responding to security reports promptly:

- **Initial Response**: Within 48 hours of receiving your report
- **Triage**: Within 5 business days we will provide an initial assessment
- **Updates**: Regular updates on our progress every 5-10 business days
- **Resolution**: Timeline depends on severity and complexity, typically within 30-90 days

## Security Measures

Omnia implements several security measures to protect users:

### Code Security

- **Static Analysis**: Automated security scanning with [gosec](https://github.com/securego/gosec) on all code changes
- **Dependency Scanning**: Automated vulnerability scanning with [Dependabot](https://github.com/dependabot) for Go modules, Docker images, and GitHub Actions
- **Code Quality**: Comprehensive linting with golangci-lint
- **Code Review**: All changes require review before merging
- **Signed Releases**: All releases are signed and checksummed

### Runtime Security

- **Input Validation**: Strict validation of all user inputs and CRD specifications
- **Secure Defaults**: Safe configuration defaults
- **Principle of Least Privilege**: Minimal required RBAC permissions
- **Audit Logging**: Security-relevant events are logged

### Kubernetes Security

- **RBAC**: Fine-grained role-based access control
- **Network Policies**: Support for network segmentation
- **Pod Security**: Non-root containers, read-only filesystems where possible
- **Secret Management**: Proper handling of sensitive credentials

### Infrastructure Security

- **Secure Development**: Development follows secure coding practices with pre-commit security checks
- **CI/CD Security**: Build pipelines use secure practices and isolated environments
- **Access Controls**: Multi-factor authentication and role-based access controls
- **Automated Updates**: Dependabot automatically creates PRs for dependency updates weekly
- **Regular Reviews**: Security updates are prioritized and reviewed promptly

## Security Considerations for Users

When using Omnia, consider the following security best practices:

### API Keys and Credentials

- **Never commit API keys** to version control
- Use Kubernetes Secrets for sensitive credentials
- Reference secrets in AgentRuntime CRs via `providerSecretRef`
- Rotate keys regularly
- Use least-privilege API keys when possible

### Data Handling

- **Sensitive Data**: Be cautious when processing sensitive information with LLMs
- **Data Residency**: Understand where your data is processed and stored
- **Logging**: Be aware of what data might be logged during processing
- **Provider Security**: Review the security practices of your LLM providers

### Configuration Security

- **Validate Configurations**: Ensure CRD configurations are from trusted sources
- **Network Security**: Use secure connections (HTTPS/TLS) for all communications
- **Access Controls**: Implement appropriate RBAC for Omnia resources
- **Updates**: Keep Omnia updated to the latest version

### Kubernetes Deployment Security

When deploying Omnia:

- **Namespace Isolation**: Deploy agents in appropriate namespaces
- **Network Policies**: Restrict network access to only required endpoints
- **Resource Limits**: Set appropriate resource limits on agent pods
- **Pod Security Standards**: Follow Kubernetes pod security standards

## Vulnerability Disclosure Policy

### Our Commitment

- We will work with security researchers to understand and fix reported vulnerabilities
- We will provide credit to researchers who report vulnerabilities responsibly
- We will not take legal action against researchers who follow this policy

### Researcher Guidelines

To be eligible for recognition:

- Follow responsible disclosure practices
- Do not access data that isn't your own
- Do not perform actions that could harm the service or other users
- Do not use social engineering against our employees or contractors
- Provide sufficient detail to reproduce the vulnerability

### Public Disclosure

Once a vulnerability is fixed:

1. We will publish a security advisory with details about the issue
2. We will credit the researcher(s) who reported the vulnerability (unless they prefer anonymity)
3. We may coordinate with the researcher on the timing of public disclosure

## Security Resources

- **Security Advisories**: [GitHub Security Advisories](https://github.com/AltairaLabs/Omnia/security/advisories)
- **Security Contact**: [security@altairalabs.ai](mailto:security@altairalabs.ai)
- **General Contact**: [conduct@altairalabs.ai](mailto:conduct@altairalabs.ai)

## Compliance and Standards

Omnia aims to follow industry security standards and best practices:

- **OWASP Guidelines**: Following OWASP secure coding practices
- **Supply Chain Security**: Using SLSA framework principles
- **OpenSSF**: Following Open Source Security Foundation guidelines
- **CVE Process**: Participating in CVE assignment for disclosed vulnerabilities

## Security Updates

Security updates are distributed through:

- **GitHub Releases**: Tagged releases with security fixes
- **Security Advisories**: GitHub security advisories for critical issues
- **Container Images**: Updated images in container registry
- **Helm Charts**: Updated chart versions
- **Documentation**: Updated security documentation and guidelines
- **Community Channels**: Announcements in community forums and discussions

## Automated Security Tools

Omnia uses the following automated security tools:

- **[gosec](https://github.com/securego/gosec)**: Go security scanner that inspects source code for security problems
  - Runs locally via `make security-scan`
  - Integrated into CI pipeline
  - Checks for common security issues like SQL injection, command injection, weak crypto, etc.

- **[Dependabot](https://github.com/dependabot)**: Automated dependency updates and vulnerability scanning
  - Monitors all Go modules, Docker images, and GitHub Actions
  - Creates pull requests for security updates and version bumps
  - Runs weekly on Mondays at 09:00 UTC

- **[golangci-lint](https://golangci-lint.run/)**: Comprehensive Go linter including staticcheck
  - Includes gosec as one of its enabled linters
  - Runs on all PRs and pre-commit hooks

---

**Last Updated**: December 28, 2025
**Next Review**: March 28, 2026

For questions about this security policy, contact: [security@altairalabs.ai](mailto:security@altairalabs.ai)
