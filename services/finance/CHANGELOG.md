# Changelog

## [0.6.0](https://github.com/ilramdhan/goapps-backend/compare/finance-service/v0.5.0...finance-service/v0.6.0) (2026-04-07)


### Features

* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([0b2c1ae](https://github.com/ilramdhan/goapps-backend/commit/0b2c1aeee96f150f60bffac2b3c17f059d05c1df))
* Add `ActiveFilter` enum for UOM queries, make `UpdateUOMRequest` fields optional, clarify `uom_code` immutability, and pin Makefile tool versions ([39f4bf3](https://github.com/ilramdhan/goapps-backend/commit/39f4bf3126c725aba80ee6de4b6300d2b3f5f11d))
* Add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([c64afe0](https://github.com/ilramdhan/goapps-backend/commit/c64afe0b5782a216d2b3a85f66ecf165a611213c))
* Embed `swagger.json` into the binary and serve it directly from memory. ([d691b45](https://github.com/ilramdhan/goapps-backend/commit/d691b45c3a2cd45da3070b9dbe78b020b304c9a4))
* Embed `swagger.json` into the binary and serve it directly from memory. ([638befc](https://github.com/ilramdhan/goapps-backend/commit/638befcb36556af3674279d2c34f5a81681b665b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/ilramdhan/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([a1ad398](https://github.com/ilramdhan/goapps-backend/commit/a1ad3980e6594d80e4154b0ecf4dfcb8f42f09c1))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/ilramdhan/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/ilramdhan/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/ilramdhan/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))
* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/ilramdhan/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([f3dac2a](https://github.com/ilramdhan/goapps-backend/commit/f3dac2a846178b0ebd80443b564b6182795c360b))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([a080253](https://github.com/ilramdhan/goapps-backend/commit/a080253986e0700c0ef996eb360ce5722273c406))
* Implement Raw Material Category (RMCategory) management within the finance service. ([c10603a](https://github.com/ilramdhan/goapps-backend/commit/c10603a71e3760a309da86b58725f93c07d6b9d5))
* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/ilramdhan/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))
* Implement the initial Identity and Access Management (IAM) serv… ([f950837](https://github.com/ilramdhan/goapps-backend/commit/f9508371b9f2a9106e8b6803ec8200affa44737f))
* Implement the initial Identity and Access Management (IAM) service with migrations, application logic, infrastructure, and gRPC delivery. ([96fe69f](https://github.com/ilramdhan/goapps-backend/commit/96fe69f34b0545fc6bd07b337e36ddf61812ee5d))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([b8238c1](https://github.com/ilramdhan/goapps-backend/commit/b8238c1e207695fafd0f916e8ddcfb4c5a5d5caa))
* Implement the new finance service for Unit of Measurement (UOM) management with full CRUD, import/export, and infrastructure components. ([58c1cee](https://github.com/ilramdhan/goapps-backend/commit/58c1cee8c5763e4f5384909bcfb3e95f4c5bb10b))
* Introduce finance service seed and migrate jobs, and add an infrastructure stability guide. ([41fa0fc](https://github.com/ilramdhan/goapps-backend/commit/41fa0fcebbb2593fa0456be73039079a97be2a4d))


### Bug Fixes

* **ci:** add `test-ci-local` command for running integration tests and make audit log index creation idempotent. ([1120a45](https://github.com/ilramdhan/goapps-backend/commit/1120a453016c6715162275bef5307b5b4b3f54bf))
* **finance:** migrate to pgx driver for SCRAM-SHA-256 support ([e914972](https://github.com/ilramdhan/goapps-backend/commit/e9149724451a53f2bbd0f538d076e9feff7a9583))
* **finance:** resolve golangci-lint v2 errors for parameter module ([96996cf](https://github.com/ilramdhan/goapps-backend/commit/96996cf6fb3a2e4faaf6b12f57cbd0cb962b96f8))
* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/ilramdhan/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/ilramdhan/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

## [0.5.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.4.0...finance-service/v0.5.0) (2026-03-20)


### Features

* Implement session idle timeout, add comprehensive E2E tests, and include a backend run guide. ([5610f2c](https://github.com/mutugading/goapps-backend/commit/5610f2cc28d10f2c2a52db56fb24f539c98962a5))

## [0.4.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.3.0...finance-service/v0.4.0) (2026-02-09)


### Features

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([55c70c8](https://github.com/mutugading/goapps-backend/commit/55c70c82a410ec6dbc328a1f0c1bec7609c6b84d))


### Bug Fixes

* Implement IAM audit log APIs, enhance Swagger documentation with security and persistent authorization, and add a swagger merge script. ([3387841](https://github.com/mutugading/goapps-backend/commit/3387841280e082eded628ce235998d5e4721eccf))

## [0.3.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.2.0...finance-service/v0.3.0) (2026-02-09)


### Features

* Implement 2FA recovery codes in IAM, add gRPC authentication and permission interceptors to IAM and Finance, and update local development infrastructure. ([c82c258](https://github.com/mutugading/goapps-backend/commit/c82c258bd39279f31789b9635367b2e4cb34d19b))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([f3dac2a](https://github.com/mutugading/goapps-backend/commit/f3dac2a846178b0ebd80443b564b6182795c360b))
* Implement JWT authentication in UOM E2E tests and update the CI workflow to provide the necessary secret. ([a080253](https://github.com/mutugading/goapps-backend/commit/a080253986e0700c0ef996eb360ce5722273c406))

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
