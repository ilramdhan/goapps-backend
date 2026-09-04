# Changelog

## [0.5.0](https://github.com/ilramdhan/goapps-backend/compare/ppc-service/v0.4.0...ppc-service/v0.5.0) (2026-09-04)


### Features

* **ppc,finance:** add month-end carries, WO lot/allocation enhancements, MB param refreeze, and calc job fixes ([a9d39bf](https://github.com/ilramdhan/goapps-backend/commit/a9d39bfa83320fd989b9ff9793e48ad7226ac5d4))
* **ppc:** add Production Planning & Control service ([3454f5f](https://github.com/ilramdhan/goapps-backend/commit/3454f5f3eafb1f78dc2a74e59ce1c98427409725))
* **ppc:** add the Production Planning & Control service ([b8be505](https://github.com/ilramdhan/goapps-backend/commit/b8be5053c61fa8e4d77e714d1a7b6c81f1fe4053))
* **ppc:** carry an unfinished plan item into the next month ([3813e02](https://github.com/ilramdhan/goapps-backend/commit/3813e0253178bcc2a01d2e932536483b06435f02))
* **ppc:** carry an unfinished work order into a later month ([2c7e23a](https://github.com/ilramdhan/goapps-backend/commit/2c7e23a03d185547fcfa15a3d733dc18a7ec9b11))
* **ppc:** decorate demands with resolved customer code/name ([e69716b](https://github.com/ilramdhan/goapps-backend/commit/e69716b31093298efed40278ae15bd5a0457d981))
* **ppc:** Decorate Demands with Resolved Customer Details & Refactor Labels ([04d4cd2](https://github.com/ilramdhan/goapps-backend/commit/04d4cd2fa0061a4d30ff86cba56e15bd49064f24))
* **ppc:** name the missing input when lot generation fails, and write a WO atomically ([7f6f60a](https://github.com/ilramdhan/goapps-backend/commit/7f6f60aa52207e8a80bb2546b3d8c75273b1c233))
* **ppc:** name the RM on every work-order allocation view ([2bb3e5d](https://github.com/ilramdhan/goapps-backend/commit/2bb3e5d6073ec85c7654df779b6f1215b2a9375e))


### Bug Fixes

* **build:** pin golangci-lint to v2.3.0 to match CI ([c00c906](https://github.com/ilramdhan/goapps-backend/commit/c00c906be743b87b9cb50a67324af76c1ba7b468))
* **ppc:** add the deployments compose its Makefile already expects ([6e97d2c](https://github.com/ilramdhan/goapps-backend/commit/6e97d2cf99d995a6661459aedba602173113d203))
* **ppc:** gate the workflow demo seeder to development only ([e8b6c63](https://github.com/ilramdhan/goapps-backend/commit/e8b6c63f5ffcc4c5b6972de86b5fd52b760d4974))
* **ppc:** surface finance BaseResponse refusals instead of reading past them ([01bb437](https://github.com/ilramdhan/goapps-backend/commit/01bb4376d6089162156a1b81773368eb1c746475))
* **ppc:** warn when a staging resolve pass returns no resolutions at all ([37a94c6](https://github.com/ilramdhan/goapps-backend/commit/37a94c684697029d13f855c1620a76644b1a3b0f))
* resolve Docker volume paths, IAM migration conflict, linter version, and PPC compose ([733bc38](https://github.com/ilramdhan/goapps-backend/commit/733bc38f102c94750b45a9c947ce28632a1565eb))
* **test-ppc:** time-bomb di test, bukan bug produksi dan bukan regresi dari kerja deployment-readiness. ([33daed0](https://github.com/ilramdhan/goapps-backend/commit/33daed06fb65752620e173a4ca2a45b351a03508))

## [0.4.0](https://github.com/mutugading/goapps-backend/compare/ppc-service/v0.3.0...ppc-service/v0.4.0) (2026-08-29)


### Features

* **ppc,finance:** add month-end carries, WO lot/allocation enhancements, MB param refreeze, and calc job fixes ([a9d39bf](https://github.com/mutugading/goapps-backend/commit/a9d39bfa83320fd989b9ff9793e48ad7226ac5d4))
* **ppc:** carry an unfinished plan item into the next month ([3813e02](https://github.com/mutugading/goapps-backend/commit/3813e0253178bcc2a01d2e932536483b06435f02))
* **ppc:** carry an unfinished work order into a later month ([2c7e23a](https://github.com/mutugading/goapps-backend/commit/2c7e23a03d185547fcfa15a3d733dc18a7ec9b11))
* **ppc:** name the missing input when lot generation fails, and write a WO atomically ([7f6f60a](https://github.com/mutugading/goapps-backend/commit/7f6f60aa52207e8a80bb2546b3d8c75273b1c233))
* **ppc:** name the RM on every work-order allocation view ([2bb3e5d](https://github.com/mutugading/goapps-backend/commit/2bb3e5d6073ec85c7654df779b6f1215b2a9375e))


### Bug Fixes

* **build:** pin golangci-lint to v2.3.0 to match CI ([c00c906](https://github.com/mutugading/goapps-backend/commit/c00c906be743b87b9cb50a67324af76c1ba7b468))
* **ppc:** add the deployments compose its Makefile already expects ([6e97d2c](https://github.com/mutugading/goapps-backend/commit/6e97d2cf99d995a6661459aedba602173113d203))
* **ppc:** surface finance BaseResponse refusals instead of reading past them ([01bb437](https://github.com/mutugading/goapps-backend/commit/01bb4376d6089162156a1b81773368eb1c746475))
* **ppc:** warn when a staging resolve pass returns no resolutions at all ([37a94c6](https://github.com/mutugading/goapps-backend/commit/37a94c684697029d13f855c1620a76644b1a3b0f))
* resolve Docker volume paths, IAM migration conflict, linter version, and PPC compose ([733bc38](https://github.com/mutugading/goapps-backend/commit/733bc38f102c94750b45a9c947ce28632a1565eb))

## [0.3.0](https://github.com/mutugading/goapps-backend/compare/ppc-service/v0.2.0...ppc-service/v0.3.0) (2026-07-30)


### Features

* **ppc:** decorate demands with resolved customer code/name ([e69716b](https://github.com/mutugading/goapps-backend/commit/e69716b31093298efed40278ae15bd5a0457d981))
* **ppc:** Decorate Demands with Resolved Customer Details & Refactor Labels ([04d4cd2](https://github.com/mutugading/goapps-backend/commit/04d4cd2fa0061a4d30ff86cba56e15bd49064f24))

## [0.2.0](https://github.com/mutugading/goapps-backend/compare/ppc-service/v0.1.0...ppc-service/v0.2.0) (2026-07-30)


### Features

* **ppc:** add Production Planning & Control service ([3454f5f](https://github.com/mutugading/goapps-backend/commit/3454f5f3eafb1f78dc2a74e59ce1c98427409725))
* **ppc:** add the Production Planning & Control service ([b8be505](https://github.com/mutugading/goapps-backend/commit/b8be5053c61fa8e4d77e714d1a7b6c81f1fe4053))


### Bug Fixes

* **ppc:** gate the workflow demo seeder to development only ([e8b6c63](https://github.com/mutugading/goapps-backend/commit/e8b6c63f5ffcc4c5b6972de86b5fd52b760d4974))
* **test-ppc:** time-bomb di test, bukan bug produksi dan bukan regresi dari kerja deployment-readiness. ([33daed0](https://github.com/mutugading/goapps-backend/commit/33daed06fb65752620e173a4ca2a45b351a03508))
