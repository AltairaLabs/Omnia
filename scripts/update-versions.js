#!/usr/bin/env node
/**
 * Updates docs/versions.json with the current release version.
 *
 * Usage: node scripts/update-versions.js <version> <is_prerelease> [needs_archive] [archive_minor] [archive_path]
 *
 * The script sets `current` to the released version and optionally archives
 * the previous version when a minor version changes.
 *
 * Examples:
 *   node scripts/update-versions.js 0.2.0 false                    # Stable release, no archive
 *   node scripts/update-versions.js 0.3.0 false true 0.2 v0-2      # Archive 0.2 when releasing 0.3
 */

const fs = require('fs');
const path = require('path');

const VERSIONS_FILE = path.join(__dirname, '..', 'docs', 'versions.json');
const PUBLIC_VERSIONS_FILE = path.join(__dirname, '..', 'docs', 'public', 'versions.json');

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

  if (args.length < 2) {
    console.error('Usage: node update-versions.js <version> <is_prerelease> [needs_archive] [archive_minor] [archive_path]');
    console.error('Example: node update-versions.js 0.3.0 false true 0.2 v0-2');
    process.exit(1);
  }

  const version = args[0];
  const isPrerelease = args[1] === 'true';
  const needsArchive = args[2] === 'true';
  const archiveMinor = args[3] || '';
  const archivePath = args[4] || '';

  const parsed = parseVersion(version);

  console.log(`Updating versions.json for ${version} (prerelease: ${isPrerelease})`);
  if (needsArchive) {
    console.log(`Archiving previous version ${archiveMinor} at /${archivePath}/`);
  }

  // Read existing versions.json
  let data;
  try {
    data = JSON.parse(fs.readFileSync(VERSIONS_FILE, 'utf8'));
  } catch (err) {
    console.error(`Error reading ${VERSIONS_FILE}:`, err.message);
    process.exit(1);
  }

  // Ensure the structure exists
  if (!('current' in data)) {
    data = { current: null, archived: [] };
  }
  if (!Array.isArray(data.archived)) {
    data.archived = [];
  }

  // Archive the previous version if needed
  if (needsArchive && data.current && archiveMinor && archivePath) {
    const archivedVersion = {
      version: archiveMinor,
      fullVersion: data.current.fullVersion,
      label: `v${archiveMinor}`,
      path: `/${archivePath}/`,
      released: data.current.released,
      status: 'archived',
      eol: new Date().toISOString(),
      helmChart: data.current.helmChart,
      dockerImage: data.current.dockerImage,
    };

    // Add to archived array (avoid duplicates)
    const existingIndex = data.archived.findIndex(a => a.version === archiveMinor);
    if (existingIndex >= 0) {
      data.archived[existingIndex] = archivedVersion;
      console.log(`Updated existing archive entry for ${archiveMinor}`);
    } else {
      data.archived.unshift(archivedVersion); // Add to front (newest first)
      console.log(`Added ${archiveMinor} to archived versions`);
    }
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
  const output = JSON.stringify(data, null, 2) + '\n';
  fs.writeFileSync(VERSIONS_FILE, output);
  console.log(`Updated ${VERSIONS_FILE}`);

  // Also copy to public folder for serving
  fs.mkdirSync(path.dirname(PUBLIC_VERSIONS_FILE), { recursive: true });
  fs.writeFileSync(PUBLIC_VERSIONS_FILE, output);
  console.log(`Updated ${PUBLIC_VERSIONS_FILE}`);

  console.log(JSON.stringify(data, null, 2));
}

main();
