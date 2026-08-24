# Changelog

## [0.1.5](https://github.com/woodleighschool/onomazo/compare/0.1.4...0.1.5) (2026-08-23)


### Bug Fixes

* align daemon runtime logging ([7760197](https://github.com/woodleighschool/onomazo/commit/77601979c2fc5ae77568c1830f32927b8968a21a))


### Code Refactoring

* align runtime configuration ([5a8fd20](https://github.com/woodleighschool/onomazo/commit/5a8fd2015514e41fea18c6bd670b259374be6282))
* use typed error extraction ([918b119](https://github.com/woodleighschool/onomazo/commit/918b1192c5c51be7a0871549b1b7e679e7743e32))


### Miscellaneous Chores

* align ignore rules ([1c69137](https://github.com/woodleighschool/onomazo/commit/1c6913702523df3c7dd527f058647b07b4e48b6b))
* align repository conventions ([a15ae2c](https://github.com/woodleighschool/onomazo/commit/a15ae2cc1fd9a289ef2e0521e403a4aa75060759))
* **release-please:** sync configuration ([88c3ad4](https://github.com/woodleighschool/onomazo/commit/88c3ad41c3af3f20a99593d64ed302e2fbad37cc))

## [0.1.4](https://github.com/woodleighschool/onomazo/compare/0.1.3...0.1.4) (2026-08-21)


### Features

* **container:** update image golang (1.26.6 → 1.27.0) ([#39](https://github.com/woodleighschool/onomazo/issues/39)) ([48fa6bb](https://github.com/woodleighschool/onomazo/commit/48fa6bb3677b621aebd4962b96483b9bb61d11ff))


### Documentation

* document cwd config default ([ffbebfb](https://github.com/woodleighschool/onomazo/commit/ffbebfb413bc02dbb4e1b194c4c604f604e99419))

## [0.1.3](https://github.com/woodleighschool/onomazo/compare/0.1.2...0.1.3) (2026-08-17)


### Bug Fixes

* **tooling:** group toolchain updates ([58ae5a4](https://github.com/woodleighschool/onomazo/commit/58ae5a4e50dc77e4b65cf1f692e8e2679023c8d3))

## [0.1.2](https://github.com/woodleighschool/onomazo/compare/0.1.1...0.1.2) (2026-08-12)


### Features

* **deps:** update module github.com/google/cel-go (v0.30.0 → v0.31.0) ([#34](https://github.com/woodleighschool/onomazo/issues/34)) ([e2c1d8c](https://github.com/woodleighschool/onomazo/commit/e2c1d8c289d37aa7336648320551a305f8bac604))
* **deps:** update module github.com/microsoftgraph/msgraph-sdk-go (v1.100.0 → v1.101.0) ([#33](https://github.com/woodleighschool/onomazo/issues/33)) ([e985e6e](https://github.com/woodleighschool/onomazo/commit/e985e6e26a765cbdb3827cc1b7f49f6efab4146e))


### Bug Fixes

* **renovate:** wait for complete toolchain groups ([7b228b8](https://github.com/woodleighschool/onomazo/commit/7b228b8c09983fc6aa52b8ca945fd4fc3498c6e3))

## [0.1.1](https://github.com/woodleighschool/onomazo/compare/0.1.0...0.1.1) (2026-08-04)


### Bug Fixes

* **ci:** disable automatic mise installs ([14948bf](https://github.com/woodleighschool/onomazo/commit/14948bfa33368dd44b11587c915767712c8e7fdb))
* **state:** stop retrying accepted renames ([a412bd6](https://github.com/woodleighschool/onomazo/commit/a412bd622b249d7d8712e24a448bfd4a708069d8))

## [0.1.0](https://github.com/woodleighschool/onomazo/compare/0.0.3...0.1.0) (2026-08-04)


### ⚠ BREAKING CHANGES

* **logging:** quiet repeated pending renames

### Bug Fixes

* **config:** remove bootstrap SHA ([5cceba9](https://github.com/woodleighschool/onomazo/commit/5cceba9f5646d289843c53e341078139eeff858b))
* **logging:** quiet repeated pending renames ([996f507](https://github.com/woodleighschool/onomazo/commit/996f5075dc7fc08568049a366a4855ac11423a7e))

## [0.0.3](https://github.com/woodleighschool/onomazo/compare/0.0.2...0.0.3) (2026-08-03)


### Features

* **config:** support ordered configuration overlays ([bb8209e](https://github.com/woodleighschool/onomazo/commit/bb8209e294c86ed02c7e816a4f94e663da5f1005))

## [0.0.2](https://github.com/woodleighschool/onomazo/compare/0.0.1...0.0.2) (2026-08-02)


### Features

* add Intune and Entra providers ([d6ca54a](https://github.com/woodleighschool/onomazo/commit/d6ca54a561b52554d9ffa8a3ef4499cd7c11ca90))
* add Jamf device provider ([04f062e](https://github.com/woodleighschool/onomazo/commit/04f062ee1fd7ba2db19595a5858e5ce445dc2fbd))
* build deterministic rename plans ([cbe5030](https://github.com/woodleighschool/onomazo/commit/cbe5030c7c8cf5cb1aae838964db22b5d68e77e8))
* columnize plan output ([e68dfe3](https://github.com/woodleighschool/onomazo/commit/e68dfe31bf9b1a07c1c195eabc691de27e6672c0))
* **container:** update image golang (1.25 → 1.26) ([#24](https://github.com/woodleighschool/onomazo/issues/24)) ([77de9a5](https://github.com/woodleighschool/onomazo/commit/77de9a5742c2b2a956952e7a0331b155dc2cfe7f))
* **deps:** update module github.com/azure/azure-sdk-for-go/sdk/azidentity (v1.13.1 → v1.14.0) ([#13](https://github.com/woodleighschool/onomazo/issues/13)) ([6421330](https://github.com/woodleighschool/onomazo/commit/6421330cb8d8fe4bbd7902636f37c1d5a4184449))
* **deps:** update module github.com/microsoftgraph/msgraph-sdk-go (v1.89.0 → v1.100.0) ([#14](https://github.com/woodleighschool/onomazo/issues/14)) ([4c2af15](https://github.com/woodleighschool/onomazo/commit/4c2af1594db6a34fc8683d6a46dfd455e7f22ea2))
* **deps:** update module github.com/spf13/cobra (v1.8.0 → v1.10.2) ([#15](https://github.com/woodleighschool/onomazo/issues/15)) ([c984373](https://github.com/woodleighschool/onomazo/commit/c984373e5eaa5f0624a474218f1b50093f77a5e1))
* **deps:** update module github.com/spf13/viper (v1.17.0 → v1.21.0) ([#16](https://github.com/woodleighschool/onomazo/issues/16)) ([5984d4d](https://github.com/woodleighschool/onomazo/commit/5984d4d38c4612a8f2fe789df36384db6728ebfb))
* persist rename intentions ([c80fbfc](https://github.com/woodleighschool/onomazo/commit/c80fbfc5d7ba46f1cc9936073dc3e61f4ef40c78))
* reconcile managed device names ([630a1ec](https://github.com/woodleighschool/onomazo/commit/630a1ecd34f941c20fa2028544433ec164e87c6c))
* validate v1 naming configuration ([d3a3d5a](https://github.com/woodleighschool/onomazo/commit/d3a3d5a02b4eab7e62a40866fe5387f860690634))


### Bug Fixes

* **deps:** update module github.com/microsoft/kiota-abstractions-go (v1.9.3 → v1.9.4) ([#12](https://github.com/woodleighschool/onomazo/issues/12)) ([065227f](https://github.com/woodleighschool/onomazo/commit/065227f38f237e24cfb093534bd7655570252188))
* lint nags ([ac1bbf6](https://github.com/woodleighschool/onomazo/commit/ac1bbf64fa9607a7a32a2354433442053ceaf73d))
* namespace provider device IDs ([97e9470](https://github.com/woodleighschool/onomazo/commit/97e94707cd70c2547c0132b43d3193aee3472688))
* start collision suffixes at one ([aecf50a](https://github.com/woodleighschool/onomazo/commit/aecf50a0e8bbbcab24297fb7e8a0c74cdd445657))


### Documentation

* add repository agent guidance ([7ba9892](https://github.com/woodleighschool/onomazo/commit/7ba9892aea3809cd9bcfe8cb6e1fa71102efa518))
