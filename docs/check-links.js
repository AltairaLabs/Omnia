#!/usr/bin/env node
import { LinkChecker } from 'linkinator';
import { spawn } from 'child_process';

// Use a dedicated port to avoid collisions with other local dev servers.
const PORT = process.env.LINK_CHECK_PORT || '4329';

// Start preview server on a dedicated port.
console.log(`🚀 Starting preview server on port ${PORT}...`);
const server = spawn('npx', ['astro', 'preview', '--port', PORT], {
  detached: true,
  stdio: 'ignore'
});

// Wait for server to be ready
await new Promise(resolve => setTimeout(resolve, 4000));

try {
  console.log('🔍 Checking all links...\n');

  const checker = new LinkChecker();
  const result = await checker.check({
    path: `http://localhost:${PORT}/`,
    recurse: true,
    timeout: 10000,
  });

  console.log(`\n📊 Total links checked: ${result.links.length}\n`);

  // Just filter broken links - keep it simple
  const brokenLinks = result.links.filter(link => link.state === 'BROKEN');

  if (brokenLinks.length === 0) {
    console.log('✅ No broken links found!');
    process.exit(0);
  }

  // Group by broken URL
  const linksByUrl = new Map();
  for (const link of brokenLinks) {
    if (!linksByUrl.has(link.url)) {
      linksByUrl.set(link.url, []);
    }
    linksByUrl.get(link.url).push(link.parent);
  }

  // Separate internal and external
  const internal = [];
  const external = [];

  for (const [url, parents] of linksByUrl) {
    const entry = { url, parents: [...new Set(parents)] };
    if (url.startsWith(`http://localhost:${PORT}`) || url.startsWith('/') || url.startsWith('https://omnia.altairalabs.ai')) {
      internal.push(entry);
    } else {
      external.push(entry);
    }
  }

  console.log(`❌ Found ${brokenLinks.length} broken links (${linksByUrl.size} unique URLs)\n`);

  if (internal.length > 0) {
    console.log(`🔴 INTERNAL BROKEN LINKS (${internal.length}):\n`);

    for (let i = 0; i < internal.length; i++) {
      const { url, parents } = internal[i];
      console.log(`  [404] ${url}`);
      console.log(`      Found on ${parents.length} page(s):`);
      parents.slice(0, 3).forEach(p => console.log(`        - ${p}`));
      if (parents.length > 3) {
        console.log(`        ... and ${parents.length - 3} more`);
      }
      console.log('');
    }
  }

  if (external.length > 0) {
    console.log(`\n🌐 EXTERNAL BROKEN LINKS (${external.length}):\n`);

    for (const { url, parents } of external) {
      console.log(`  [404] ${url}`);
      console.log(`      Found on ${parents.length} page(s):`);
      parents.slice(0, 3).forEach(p => console.log(`        - ${p}`));
      if (parents.length > 3) {
        console.log(`        ... and ${parents.length - 3} more`);
      }
      console.log('');
    }
  }

  process.exit(1);
} catch (error) {
  console.error('Error:', error.message);
  process.exit(1);
} finally {
  try {
    process.kill(-server.pid);
  } catch (e) {
    // Ignore
  }
}
