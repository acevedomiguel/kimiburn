import assert from "node:assert/strict";
import test from "node:test";

import { validateModuleVersion } from "../../scripts/check-module-version.mjs";

const modulePath = "github.com/acevedomiguel/kimiburn";

test("an unsuffixed module accepts only v1 releases", () => {
  validateModuleVersion(modulePath, "1.9.0");
  validateModuleVersion(modulePath, "v1.0.0");
  validateModuleVersion(modulePath, "1.2.3-rc.1+build.5");
  assert.throws(() => validateModuleVersion(modulePath, "2.0.0"), /expects v1 releases, received 2\.0\.0/);
  assert.throws(() => validateModuleVersion(modulePath, "0.9.0"), /expects v1 releases, received 0\.9\.0/);
});

test("a versioned module accepts only its matching major", () => {
  validateModuleVersion(`${modulePath}/v2`, "2.0.0");
  validateModuleVersion(`${modulePath}/v10`, "10.1.0");
  assert.throws(() => validateModuleVersion(`${modulePath}/v2`, "1.9.0"), /expects v2 releases, received 1\.9\.0/);
});

test("malformed versions and module paths are rejected", () => {
  for (const version of ["not-semver", "1.2", "v1.2.3.4", ""]) {
    assert.throws(() => validateModuleVersion(modulePath, version), /invalid semantic version/);
  }
  assert.throws(() => validateModuleVersion("", "1.2.3"), /module path is required/);
});
