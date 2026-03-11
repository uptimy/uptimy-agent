# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Uptimy Agent, please report it
responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email: **security@upti.my**

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Response Timeline

- **Acknowledgement**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix and disclosure**: Coordinated with reporter

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

## Security Considerations

The Uptimy Agent executes repair actions on infrastructure. Key security
properties:

- **Guardrails**: All repair actions must be explicitly allowed in configuration
- **No arbitrary execution**: The agent does not support running arbitrary
  commands; only registered, deterministic actions are permitted
- **TLS support**: gRPC connections to the control plane support TLS
- **Secret injection**: Use environment variable expansion (`${VAR}`) in
  configuration files instead of hardcoding tokens
