#!/usr/bin/env node
/**
 * Updates docs/versions.json with a new release version.
 *
 * Usage: node scripts/update-versions.js <version> [is_prerelease]
 *
 * Examples:
 *   node scripts/update-versions.js 0.2.0 false     # Stable release
 *   node scripts/update-versions.js 0.3.0-beta.1 true  # Pre-release
 */

const fs = require('fs');
const path = require('path');

const VERSIONS_FILE = path.join(__dirname, '..', 'docs', 'versions.json');

function parseVersion(version) {
  // Remove 'v' prefix if present
  const v = version.replace(/^v/, '');
  const [base, prerelease] = v.split('-');
  const [major, minor, patch] = base.split('.').map(Number);

  return {
    full: v,
    major,
    minor,
    patch,
    prerelease: prerelease || null,
    minorVersion: `${major}.${minor}`,
  };
}

function main() {
  const args = process.argv.slice(2);

  if (args.length < 1) {
    console.error('Usage: node update-versions.js <version> [is_prerelease]');
    console.error('Example: node update-versions.js 0.2.0 false');
    process.exit(1);
  }

  const version = args[0];
  const isPrerelease = args[1] === 'true';
  const parsed = parseVersion(version);

  console.log(`Updating versions.json for ${version} (prerelease: ${isPrerelease})`);

  // Read existing versions.json
  let data;
  try {
    data = JSON.parse(fs.readFileSync(VERSIONS_FILE, 'utf8'));
  } catch (err) {
    console.error(`Error reading ${VERSIONS_FILE}:`, err.message);
    process.exit(1);
  }

  // Check if this minor version already exists
  const existingIndex = data.versions.findIndex(
    (v) => v.version === parsed.minorVersion
  );

  const newEntry = {
    version: parsed.minorVersion,
    fullVersion: parsed.full,
    label: isPrerelease
      ? `v${parsed.minorVersion} (${parsed.prerelease || 'Pre-release'})`
      : `v${parsed.minorVersion}${!isPrerelease ? ' (Latest)' : ''}`,
    // Versioned docs are at /v0-2/, unversioned (latest dev) at /
    // Use hyphen instead of dot to avoid URL encoding issues
    path: `/v${parsed.major}-${parsed.minor}/`,
    released: new Date().toISOString(),
    status: isPrerelease ? 'prerelease' : 'stable',
    eol: null,
    helmChart: `oci://ghcr.io/altairalabs/charts/omnia:${parsed.full}`,
    dockerImage: `ghcr.io/altairalabs/omnia:${parsed.full}`,
  };

  if (existingIndex >= 0) {
    // Update existing entry
    data.versions[existingIndex] = newEntry;
    console.log(`Updated existing entry for v${parsed.minorVersion}`);
  } else {
    // Add new entry at the beginning
    data.versions.unshift(newEntry);
    console.log(`Added new entry for v${parsed.minorVersion}`);
  }

  // Update latest pointers
  if (isPrerelease) {
    data.latestPrerelease = parsed.full;
  } else {
    data.latest = parsed.full;
    // Update labels - remove "(Latest)" from other stable versions
    data.versions.forEach((v) => {
      if (v.status === 'stable' && v.version !== parsed.minorVersion) {
        v.label = `v${v.version}`;
      }
    });
  }

  // Sort versions (newest first)
  data.versions.sort((a, b) => {
    const [aMajor, aMinor] = a.version.split('.').map(Number);
    const [bMajor, bMinor] = b.version.split('.').map(Number);
    if (bMajor !== aMajor) return bMajor - aMajor;
    return bMinor - aMinor;
  });

  // Write updated versions.json
  fs.writeFileSync(VERSIONS_FILE, JSON.stringify(data, null, 2) + '\n');
  console.log(`Updated ${VERSIONS_FILE}`);
  console.log(JSON.stringify(data, null, 2));
}

main();
