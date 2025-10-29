# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Project Overview and Developer Documentation
- @README.md

User Guides and How-To Guides
- @docs/modules/operator/partials/how-to-guide.adoc
- @docs/modules/gateway/partials/how-to-guide.adoc

Reference Documentation
- Overall Agentic Layer Architecture: https://docs.agentic-layer.ai/architecture/main/index.html

Documentation in AsciiDoc format is located in the `docs/` directory.
This folder is hosted as a separate [documentation site](https://docs.agentic-layer.ai/agent-runtime-operator/index.html).

### Project Structure

```
├── api/                  # CRD definitions and types
├── cmd/main.go           # Operator entry point
├── config/               # Kubernetes manifests and Kustomize configs
│   ├── crd/              # Custom Resource Definitions
│   ├── rbac/             # Role-based access control
│   ├── manager/          # Operator deployment
│   ├── webhook/          # Webhook configurations
│   └── samples/          # Example resources
├── docs/                 # AsciiDoc documentation
├── internal/
│   ├── controller/       # Reconciliation logic
│   └── webhook/          # Admission webhook handlers
└── test/
    ├── e2e/              # End-to-end tests
    └── utils/            # Test utilities
```
