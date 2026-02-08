# Changelog

## [0.3.0](https://github.com/ilramdhan/goapps-backend/compare/finance-service/v0.2.0...finance-service/v0.3.0) (2026-02-08)


### Features

* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([0b2c1ae](https://github.com/ilramdhan/goapps-backend/commit/0b2c1aeee96f150f60bffac2b3c17f059d05c1df))
* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([39f4bf3](https://github.com/ilramdhan/goapps-backend/commit/39f4bf3126c725aba80ee6de4b6300d2b3f5f11d))
* Add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([c64afe0](https://github.com/ilramdhan/goapps-backend/commit/c64afe0b5782a216d2b3a85f66ecf165a611213c))
* Embed `swagger.json` into the binary and serve it directly from memory. ([d691b45](https://github.com/ilramdhan/goapps-backend/commit/d691b45c3a2cd45da3070b9dbe78b020b304c9a4))
* Embed `swagger.json` into the binary and serve it directly from memory. ([638befc](https://github.com/ilramdhan/goapps-backend/commit/638befcb36556af3674279d2c34f5a81681b665b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/ilramdhan/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([a1ad398](https://github.com/ilramdhan/goapps-backend/commit/a1ad3980e6594d80e4154b0ecf4dfcb8f42f09c1))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([f3dac2a](https://github.com/ilramdhan/goapps-backend/commit/f3dac2a846178b0ebd80443b564b6182795c360b))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([a080253](https://github.com/ilramdhan/goapps-backend/commit/a080253986e0700c0ef996eb360ce5722273c406))
* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/ilramdhan/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/ilramdhan/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([b8238c1](https://github.com/ilramdhan/goapps-backend/commit/b8238c1e207695fafd0f916e8ddcfb4c5a5d5caa))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([58c1cee](https://github.com/ilramdhan/goapps-backend/commit/58c1cee8c5763e4f5384909bcfb3e95f4c5bb10b))


### Bug Fixes

* **ci:** add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([1120a45](https://github.com/ilramdhan/goapps-backend/commit/1120a453016c6715162275bef5307b5b4b3f54bf))
* **finance:** migrate to pgx driver for SCRAM-SHA-256 support ([e914972](https://github.com/ilramdhan/goapps-backend/commit/e9149724451a53f2bbd0f538d076e9feff7a9583))

## [0.2.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.1.0...finance-service/v0.2.0) (2026-02-08)


### Features

* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([0b2c1ae](https://github.com/mutugading/goapps-backend/commit/0b2c1aeee96f150f60bffac2b3c17f059d05c1df))
* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([39f4bf3](https://github.com/mutugading/goapps-backend/commit/39f4bf3126c725aba80ee6de4b6300d2b3f5f11d))
* Add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([c64afe0](https://github.com/mutugading/goapps-backend/commit/c64afe0b5782a216d2b3a85f66ecf165a611213c))
* Embed `swagger.json` into the binary and serve it directly from memory. ([d691b45](https://github.com/mutugading/goapps-backend/commit/d691b45c3a2cd45da3070b9dbe78b020b304c9a4))
* Embed `swagger.json` into the binary and serve it directly from memory. ([638befc](https://github.com/mutugading/goapps-backend/commit/638befcb36556af3674279d2c34f5a81681b665b))
* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/mutugading/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/mutugading/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([b8238c1](https://github.com/mutugading/goapps-backend/commit/b8238c1e207695fafd0f916e8ddcfb4c5a5d5caa))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([58c1cee](https://github.com/mutugading/goapps-backend/commit/58c1cee8c5763e4f5384909bcfb3e95f4c5bb10b))


### Bug Fixes

* **ci:** add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([1120a45](https://github.com/mutugading/goapps-backend/commit/1120a453016c6715162275bef5307b5b4b3f54bf))
* **finance:** migrate to pgx driver for SCRAM-SHA-256 support ([e914972](https://github.com/mutugading/goapps-backend/commit/e9149724451a53f2bbd0f538d076e9feff7a9583))
