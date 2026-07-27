// Package config no longer needs a local readNested helper — testutil.NestedMapChain
// is used instead. This file is kept as a build guard to ensure the package
// still compiles after the migration.
package config
