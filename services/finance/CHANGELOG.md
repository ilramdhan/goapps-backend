# Changelog

## [0.12.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.4...finance-service/v0.12.0) (2026-05-07)


### Features

* **export-notification:** Implement generic IAM notification system with MinIO export support ([124f619](https://github.com/mutugading/goapps-backend/commit/124f619c16e4f2ef5bc70c975fc9e40deb2e1244))
* **infra:** add MinIO storage client and IAM gRPC client wrapper ([4266c38](https://github.com/mutugading/goapps-backend/commit/4266c38f52de657555a1eb0cbaf973b1cb4a6578))
* **rm-cost:** async export to MinIO with EXPORT_READY notification emit ([3d46ba5](https://github.com/mutugading/goapps-backend/commit/3d46ba538af45513c4bb9cb8292c436cecc8ecf8))


### Bug Fixes

* **lint:** resolve golangci-lint errors and apply Copilot review feedback ([9b96fd5](https://github.com/mutugading/goapps-backend/commit/9b96fd5fc97b200577cca144b5d3656a6930f639))
* **lint:** resolve remaining gocyclo, gocognit, and errorlint failures ([962dcfd](https://github.com/mutugading/goapps-backend/commit/962dcfdad6700bb843643589396696dc7b4d4221))
* **s3:** presign against public endpoint to avoid signature mismatch ([e17a75a](https://github.com/mutugading/goapps-backend/commit/e17a75a078661e2cf29514ede49517b1790a169f))
* **storage:** presign against public endpoint to avoid signature mismatch ([287b46a](https://github.com/mutugading/goapps-backend/commit/287b46a267a65d716a04cfeaafb62f680e8f3a7b))
* **tracing:** fetch tracer lazily per-request to survive late provider init ([0186299](https://github.com/mutugading/goapps-backend/commit/018629984683afb9ba47c9f02ee198a36f835336))
* **tracing:** fetch tracer lazily per-request to survive late provider init ([788d0bc](https://github.com/mutugading/goapps-backend/commit/788d0bc6c0db634bd90430354542d21086b541a5))
* **tracing:** use otlptracegrpc.WithInsecure() to actually disable TLS ([0908955](https://github.com/mutugading/goapps-backend/commit/090895526959ecbc8deaa37a8f7fc1dc661aa8b7))

## [0.11.4](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.3...finance-service/v0.11.4) (2026-05-06)


### Bug Fixes

* **rm-cost:** document semantics of stock columns in cst_rm_cost_detail for clarity ([5874eb6](https://github.com/mutugading/goapps-backend/commit/5874eb697853391ad5ce50ee458d4ff1d60fa2bb))
* **syncdata:** enhance stock handling in FetchSourceQtyByItemGrade for accurate inventory reporting ([a0c5edd](https://github.com/mutugading/goapps-backend/commit/a0c5eddf769e16058770c2e7f55b1212abf443c0))

## [0.11.3](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.2...finance-service/v0.11.3) (2026-05-05)


### Bug Fixes

* **rm-cost:** implement V2 flag resolution for valuation and marketing in RMCost handler ([ef09750](https://github.com/mutugading/goapps-backend/commit/ef09750f2355d549bf4ea8a60fe481821ea6646f))
* **rm-cost:** implement V2 flag resolution for valuation and marketing in RMCost handler ([2afb1d1](https://github.com/mutugading/goapps-backend/commit/2afb1d1fcd650eb6c7a59d87a398dd7f9d5085d8))

## [0.11.2](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.1...finance-service/v0.11.2) (2026-05-05)


### Bug Fixes

* **ungrouped-handler:** optimize UngroupedQuery to UngroupedItemsFilter conversion by using direct type conversion ([b0e2a30](https://github.com/mutugading/goapps-backend/commit/b0e2a3078389f9efcd8559c9373db1906b3b233d))
* **ungrouped-items:** update UngroupedItems handling to support group ing monitor view with enhanced filtering and sorting options ([0611c25](https://github.com/mutugading/goapps-backend/commit/0611c250f41e2c75791f4e88f8f7e14c5f359f51))
* **ungrouped-items:** update UngroupedItems handling to support grouping monitor view with enhanced filtering and sorting options ([2909d73](https://github.com/mutugading/goapps-backend/commit/2909d73a2643811664609f1d533923a6f41b88b7))

## [0.11.1](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.11.0...finance-service/v0.11.1) (2026-05-04)


### Bug Fixes

* add search and explicit ID filters to ExportRMGroups and refactor RM Group import handler to support V2 schema ([5dbcfbd](https://github.com/mutugading/goapps-backend/commit/5dbcfbdbabcebb3663e2449d5df2f422a5f6846b))
* add search and explicit ID filters to ExportRMGroups and refactor RM Group import handler to support V2 schema ([76584a2](https://github.com/mutugading/goapps-backend/commit/76584a2ce0fc02636d2f199bf4bb92d20ef60546))
* improve error handling in RM group export and ensure proper resource cleanup in item lookup ([336c54d](https://github.com/mutugading/goapps-backend/commit/336c54d89c19f1577c4ecbc4b40e9f5789db9420))
* **test:** enhance ambiguity handling in RM group import tests for CI compatibility ([c0bc804](https://github.com/mutugading/goapps-backend/commit/c0bc8042ec2dc68deca9fbeadf6c7bfd61e0ba25))
* **test:** enhance ambiguity handling in RM group import tests for CI compatibility. ([144ba88](https://github.com/mutugading/goapps-backend/commit/144ba885642cba5dece9a9d96e2c6f88079d9ad1))

## [0.11.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.10.0...finance-service/v0.11.0) (2026-04-30)


### Features

* implement RM cost calculation v2 and expand raw material group data models with database migrations ([ffafed9](https://github.com/mutugading/goapps-backend/commit/ffafed9b5220e8b0888c522a98013569ebbf52ab))
* implement RM cost calculation v2 and expand raw material group data models with database migrations ([d8a9022](https://github.com/mutugading/goapps-backend/commit/d8a902271b371744f6cf42e01679e4a7d2d8a787))

## [0.10.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.9.1...finance-service/v0.10.0) (2026-04-22)


### Features

* implement raw material grouping and cost management modules with associated gRPC services and database migrations ([f67d111](https://github.com/mutugading/goapps-backend/commit/f67d111cb998323e80f8d3a8b9b93859227af4fa))
* implement raw material grouping and cost management modules with associated gRPC services and database migrations ([a24776a](https://github.com/mutugading/goapps-backend/commit/a24776a45003a72248a9c45c0d35dd776d23ada8))


### Bug Fixes

* centralize group head ID parsing and apply consistent formatting to colourant field labels ([cbeb463](https://github.com/mutugading/goapps-backend/commit/cbeb4630294606260fa63daab98579c32f904ea2))
* improve file handling, add linting annotations, and fix formatting across finance services ([365f51b](https://github.com/mutugading/goapps-backend/commit/365f51bd7c7885e5e4c4ef9d5ca8934241cee861))
* rename Colourant to Colorant throughout the codebase and update minor internal helpers ([25e8f5c](https://github.com/mutugading/goapps-backend/commit/25e8f5cc1cef2276107e42f9aab375634dbbb91f))

## [0.9.1](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.9.0...finance-service/v0.9.1) (2026-04-17)


### Bug Fixes

* **finance:** correct Oracle column mappings and update comments for consistency and accuracy ([8caee5d](https://github.com/mutugading/goapps-backend/commit/8caee5de40dd82417fa5040f09d95e93505b177b))
* **finance:** correct Oracle column mappings and update comments for consistency and accuracy ([6b87199](https://github.com/mutugading/goapps-backend/commit/6b871998cd18f34edfa8f45ab612d2e5fea7c91d))

## [0.9.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.8.1...finance-service/v0.9.0) (2026-04-17)


### Features

* **finance:** implement job execution tracking, Oracle sync, and RabbitMQ integration with migrations and handlers ([8f9896c](https://github.com/mutugading/goapps-backend/commit/8f9896c2de11dbb5428a71cb20430bec773a7a05))
* **finance:** implement job execution tracking, Oracle sync, and RabbitMQ integration with migrations and handlers ([02db90f](https://github.com/mutugading/goapps-backend/commit/02db90f9eee87a72611700192d10614266cd65b8))


### Bug Fixes

* **finance:** enhance Oracle sync system with improved error handling, concurrency safety, and refined data validation ([4ce67ee](https://github.com/mutugading/goapps-backend/commit/4ce67eeb620dd423b9dab6a77bfe52ad3c21c839))
* **finance:** standardize "canceled" spelling, update identifiers and comments to match proto/DB conventions ([3747f0a](https://github.com/mutugading/goapps-backend/commit/3747f0a154d62bc7558a394e241041b8d37b473a))
* **finance:** update Oracle sync procedure and schema, align period logic, and refine tests ([fe45bc0](https://github.com/mutugading/goapps-backend/commit/fe45bc07519e1ae19dded630c2c46c42304346e7))

## [0.8.1](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.8.0...finance-service/v0.8.1) (2026-04-15)


### Bug Fixes

* **chore:** add shared module copy step to Dockerfile iam and finance svc for dependency resolution ([ea159be](https://github.com/mutugading/goapps-backend/commit/ea159bee99c871929bc6dda7fc060a23c5431843))
* **chore:** add shared module copy step to Dockerfile iam and finance svc for dependency resolution ([a0e2c65](https://github.com/mutugading/goapps-backend/commit/a0e2c65578d70838ca78c6f3b833e2abd92bca22))

## [0.8.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.7.0...finance-service/v0.8.0) (2026-04-13)


### Features

* add UOM category management with CRUD, import/export, and seed migration ([29e4c43](https://github.com/mutugading/goapps-backend/commit/29e4c432fdaf2ac13f81dd548488ed6d5596c75f))
* add UOM category management with CRUD, import/export, and seed migration ([f4cbaaa](https://github.com/mutugading/goapps-backend/commit/f4cbaaaed866fec031f8a0af05bdb98970d99254))


### Bug Fixes

* **lint:** add nolint:dupl directives to category repositories and handlers ([035d9d1](https://github.com/mutugading/goapps-backend/commit/035d9d1343b0817174f562e9221f1271fce37909))
* **test:** update UOM E2E tests to use dynamic category IDs via UOMCategoryService ([6412c0f](https://github.com/mutugading/goapps-backend/commit/6412c0f97a18df609d932f933a2942ae053693cf))

## [0.7.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.6.0...finance-service/v0.7.0) (2026-04-08)


### Features

* implement formula management service with CRUD operations, gRPC definitions, and database schema ([ee9831c](https://github.com/mutugading/goapps-backend/commit/ee9831c156013d0b81b3634645934392dffef324))
* implement formula management service with CRUD operations, gRPC definitions, and database schema ([1faa10c](https://github.com/mutugading/goapps-backend/commit/1faa10ce43da4772131957ef9f53ce95987e5038))


### Bug Fixes

* enforce unique result parameters per formula and add validation constraints for description and input parameters ([5f8dea5](https://github.com/mutugading/goapps-backend/commit/5f8dea554ace536e7d949f7ac46c76ca1b1a19f5))
* update formula parsing methods to return change status alongside values ([b5e5d75](https://github.com/mutugading/goapps-backend/commit/b5e5d75abc5c228236589f4abdd5639942a38d35))

## [0.6.0](https://github.com/mutugading/goapps-backend/compare/finance-service/v0.5.0...finance-service/v0.6.0) (2026-04-07)


### Features

* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([5eb8515](https://github.com/mutugading/goapps-backend/commit/5eb8515c646e89a934bbd5694c4b9833c6accad8))
* implement CMS module with CRUD operations, database schema, and gRPC gateway support ([c0df3b4](https://github.com/mutugading/goapps-backend/commit/c0df3b47eb36a97bf1eec792ac114ffa4d9571a8))
* implement finance parameter management module with CRUD, import/export, and UI components ([9505fd1](https://github.com/mutugading/goapps-backend/commit/9505fd10daa3aedf88a101e1aea7d0dfe572bcd3))
* Implement Raw Material Category (RMCategory) management within the finance service. ([c10603a](https://github.com/mutugading/goapps-backend/commit/c10603a71e3760a309da86b58725f93c07d6b9d5))
* Introduce finance service seed and migrate jobs, and add an infrastructure stability guide. ([41fa0fc](https://github.com/mutugading/goapps-backend/commit/41fa0fcebbb2593fa0456be73039079a97be2a4d))


### Bug Fixes

* **finance:** resolve golangci-lint v2 errors for parameter module ([96996cf](https://github.com/mutugading/goapps-backend/commit/96996cf6fb3a2e4faaf6b12f57cbd0cb962b96f8))
* Seed raw material categories with IAM menu and permissions, and refactor sort order strings to constants. ([1330c63](https://github.com/mutugading/goapps-backend/commit/1330c630b279efc4302ca31758489ba44f712ad3))

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
