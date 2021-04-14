# License Checker

Our license checker CI rely on https://github.com/pivotal/LicenseFinder.

## How to add a new license?

LicenseFinder is a ruby project, so make sure you have ruby installed.

### Install the tool

```shell
gem install license_finder
```

### Add a license

```shell
license_finder  permitted_licenses add MIT --decisions_file .license/dependency_decisions.yml
```
