/**
 * Minimal stub for Node.js path module.
 * Used by Turbopack to replace path in browser builds.
 */

export const sep = '/';
export const delimiter = ':';

export const parse = (pathString) => {
  const parts = pathString.split('/');
  const base = parts.pop() || '';
  const extIdx = base.lastIndexOf('.');
  return {
    root: pathString.startsWith('/') ? '/' : '',
    dir: parts.join('/'),
    base,
    ext: extIdx > 0 ? base.slice(extIdx) : '',
    name: extIdx > 0 ? base.slice(0, extIdx) : base,
  };
};

export const join = (...parts) => {
  return parts.filter(Boolean).join('/').replaceAll(/\/+/g, '/');
};

export const resolve = (...parts) => {
  return '/' + join(...parts).replace(/^\/+/, '');
};

export const dirname = (p) => {
  const parts = p.split('/');
  parts.pop();
  return parts.join('/') || '/';
};

export const basename = (p, ext) => {
  const base = p.split('/').pop() || '';
  if (ext && base.endsWith(ext)) {
    return base.slice(0, -ext.length);
  }
  return base;
};

export const extname = (p) => {
  const base = basename(p);
  const idx = base.lastIndexOf('.');
  return idx > 0 ? base.slice(idx) : '';
};

export const isAbsolute = (p) => p.startsWith('/');

export const normalize = (p) => p.replaceAll(/\/+/g, '/');

export const relative = (from, to) => {
  // Simplified implementation
  return to;
};

export const posix = {
  sep, delimiter, parse, join, resolve, dirname, basename, extname, isAbsolute, normalize, relative
};

export const win32 = posix; // Use posix for browser

export default {
  sep, delimiter, parse, join, resolve, dirname, basename, extname, isAbsolute, normalize, relative, posix, win32
};
