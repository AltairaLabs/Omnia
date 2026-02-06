/**
 * Empty stub for Node.js built-in modules.
 * Used by Turbopack to replace Node.js modules in browser builds.
 *
 * This is needed for monaco-languageclient/vscode packages which have
 * code that conditionally requires Node.js modules.
 *
 * IMPORTANT: Objects must be extensible as vscode packages may try to add properties.
 */

// Common no-op function for any method calls
const noop = () => {};

// Export common patterns that might be accessed
export const createServer = noop;
export const connect = noop;
export const request = noop;
export const get = noop;
export const Socket = class {};
export const Server = class {};
export const constants = Object.create(null);
export const platform = () => 'browser';
export const arch = () => 'browser';
export const type = () => 'Browser';
export const release = () => '0.0.0';
export const hostname = () => 'localhost';
export const homedir = () => '/';
export const tmpdir = () => '/tmp';
export const cpus = () => [];
export const totalmem = () => 0;
export const freemem = () => 0;
export const networkInterfaces = () => ({});
export const EOL = '\n';

// Export extensible object as default - MUST be extensible for vscode packages
const defaultExport = Object.create(null);
Object.assign(defaultExport, {
  createServer, connect, request, get, Socket, Server, constants,
  platform, arch, type, release, hostname, homedir, tmpdir,
  cpus, totalmem, freemem, networkInterfaces, EOL
});
export default defaultExport;
