/**
 * Minimal stub for Node.js fs module.
 * Used by Turbopack to replace fs in browser builds.
 */

export const readFileSync = () => { throw new Error('fs.readFileSync is not available in browser'); };
export const writeFileSync = () => { throw new Error('fs.writeFileSync is not available in browser'); };
export const existsSync = () => false;
export const mkdirSync = () => {};
export const readdirSync = () => [];
export const statSync = () => ({ isDirectory: () => false, isFile: () => false });
export const unlinkSync = () => {};
export const rmdirSync = () => {};
export const readFile = (path, options, callback) => {
  const cb = typeof options === 'function' ? options : callback;
  if (cb) cb(new Error('fs.readFile is not available in browser'));
};
export const writeFile = (path, data, options, callback) => {
  const cb = typeof options === 'function' ? options : callback;
  if (cb) cb(new Error('fs.writeFile is not available in browser'));
};
export const promises = {
  readFile: () => Promise.reject(new Error('fs.promises.readFile is not available in browser')),
  writeFile: () => Promise.reject(new Error('fs.promises.writeFile is not available in browser')),
  mkdir: () => Promise.resolve(),
  readdir: () => Promise.resolve([]),
  stat: () => Promise.resolve({ isDirectory: () => false, isFile: () => false }),
  unlink: () => Promise.resolve(),
  rmdir: () => Promise.resolve(),
};

export default {
  readFileSync,
  writeFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  statSync,
  unlinkSync,
  rmdirSync,
  readFile,
  writeFile,
  promises,
};
