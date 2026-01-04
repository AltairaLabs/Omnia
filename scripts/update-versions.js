#!/usr/bin/env node
/**
 * Updates docs/versions.json with the current release version.
 *
 * Usage: node scripts/update-versions.js <version> [is_prerelease]
 *
 * The script sets `current` to the released version. Archived versions
 * are managed separately when a new minor version is released.
 *
 * Examples:
 *   node scripts/update-versions.js 0.2.0 false        # Stable release
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

  // Ensure the new structure exists
  if (!('current' in data)) {
    data = { current: null, archived: data.archived || [] };
  }

  // Build the label - show "Latest (v0.2)" for root deployment
  const statusLabel = isPrerelease ? 'Pre-release' : 'Latest';
  const label = `${statusLabel} (v${parsed.minorVersion})`;

  // Update current to the new release
  data.current = {
    version: parsed.minorVersion,
    fullVersion: parsed.full,
    label: label,
    path: '/',  // Current is always deployed at root
    released: new Date().toISOString(),
    status: isPrerelease ? 'prerelease' : 'stable',
    eol: null,
    helmChart: `oci://ghcr.io/altairalabs/charts/omnia:${parsed.full}`,
    dockerImage: `ghcr.io/altairalabs/omnia:${parsed.full}`,
  };

  // Write updated versions.json
  fs.writeFileSync(VERSIONS_FILE, JSON.stringify(data, null, 2) + '\n');
  console.log(`Updated ${VERSIONS_FILE}`);
  console.log(JSON.stringify(data, null, 2));
}

main();
