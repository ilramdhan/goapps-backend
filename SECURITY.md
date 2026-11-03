# Security Policy

## Supported Versions

Only the `main` branch and the version currently deployed from it receive security fixes. Older commits, forks, and unmerged branches are not supported.

## Reporting a Vulnerability

Please report vulnerabilities privately using GitHub Private Vulnerability Reporting: open the **Security** tab of this repository and choose **Report a vulnerability**.

Do **not** open public issues, pull requests, or discussions for security problems.

Please include a description, affected component, reproduction steps or proof of concept, and the potential impact.

## Response Timeline

- Acknowledgement within **3 business days**.
- Triage and severity assessment within **7 business days**.
- We will keep you updated on the fix progress and coordinate the release.

## Scope

- Go gRPC microservices (Finance, IAM, PPC, cost worker/orchestrator): authentication, authorization (RBAC), input handling, SQL access, secrets handling, and container images built from this repository.
- The CI workflows in this repository.

Out of scope: social engineering, denial-of-service via volumetric traffic, and findings in third-party services we do not operate.

## Disclosure Policy

We follow coordinated disclosure: please give us a reasonable time to ship a fix before any public disclosure. We will credit reporters in the advisory unless they prefer to remain anonymous.
