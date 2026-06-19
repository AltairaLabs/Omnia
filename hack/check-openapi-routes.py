#!/usr/bin/env python3
"""Guard: api/openapi/openapi.yaml must not document routes/methods that don't
exist as Next.js dashboard route handlers.

This prevents the spec from drifting into fiction (e.g. documenting an
`/api/v1/agents` API that was never implemented). The dashboard's filesystem
routes under dashboard/src/app/api/**/route.ts are the source of truth.

For every path documented in the spec:
  * the backing `route.ts` must exist (dynamic `{seg}` resolves positionally to
    the single Next.js `[...]` dir at that level — names need not match), and
  * every HTTP method documented for that path must be exported by that file.

Dependency-free (stdlib only) so it runs anywhere CI does. Exit 1 on drift.
"""
import os
import re
import sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SPEC = os.path.join(REPO_ROOT, "api", "openapi", "openapi.yaml")
APP_API = os.path.join(REPO_ROOT, "dashboard", "src", "app", "api")
METHODS = ("get", "post", "put", "delete", "patch")


def parse_spec(path):
    """Return {url_path: set(methods)} from the spec's `paths:` block."""
    paths, cur = {}, None
    in_paths = False
    for line in open(path, encoding="utf-8"):
        if re.match(r"^paths:\s*$", line):
            in_paths = True
            continue
        if in_paths and re.match(r"^\S", line):  # next top-level key
            break
        if not in_paths:
            continue
        m = re.match(r"^  (/\S+):\s*$", line)  # 2-space path key
        if m:
            cur = m.group(1)
            paths[cur] = set()
            continue
        m = re.match(r"^    (get|post|put|delete|patch):", line)  # 4-space method
        if m and cur:
            paths[cur].add(m.group(1).upper())
    return paths


def resolve_route_file(url_path):
    """Map a spec URL to its route.ts, resolving {seg} to the lone [*] dir."""
    segs = [s for s in url_path.strip("/").split("/") if s]
    if not segs or segs[0] != "api":
        return None, f"path does not start with /api: {url_path}"
    cur = APP_API
    for seg in segs[1:]:
        if seg.startswith("{") and seg.endswith("}"):
            if not os.path.isdir(cur):
                return None, f"no directory for dynamic segment under {cur}"
            dynamic = [d for d in os.listdir(cur)
                       if d.startswith("[") and d.endswith("]")]
            if len(dynamic) != 1:
                return None, (f"expected exactly one dynamic dir in {cur}, "
                              f"found {dynamic}")
            cur = os.path.join(cur, dynamic[0])
        else:
            cur = os.path.join(cur, seg)
    route = os.path.join(cur, "route.ts")
    return route, None


def exported_methods(route_file):
    """Extract HTTP methods exported by a Next.js route file."""
    text = open(route_file, encoding="utf-8").read()
    found = set()
    # export const { GET, POST } = createCollectionRoutes(...)
    for braces in re.findall(r"export\s+const\s*\{([^}]*)\}\s*=", text):
        for tok in re.split(r"[,\s]+", braces):
            if tok.upper() in (m.upper() for m in METHODS):
                found.add(tok.upper())
    # export const GET = ...   /   export async function GET(...)
    for m in re.findall(r"export\s+(?:async\s+function|const)\s+(GET|POST|PUT|DELETE|PATCH)\b", text):
        found.add(m)
    return found


def main():
    if not os.path.isfile(SPEC):
        print(f"spec not found: {SPEC}", file=sys.stderr)
        return 1
    errors = []
    spec_paths = parse_spec(SPEC)
    if not spec_paths:
        print("no paths parsed from spec — parser/spec mismatch", file=sys.stderr)
        return 1
    for url, methods in sorted(spec_paths.items()):
        route, err = resolve_route_file(url)
        if err:
            errors.append(f"{url}: {err}")
            continue
        if not os.path.isfile(route):
            rel = os.path.relpath(route, REPO_ROOT)
            errors.append(f"{url}: documented but no route handler at {rel}")
            continue
        exported = exported_methods(route)
        for m in sorted(methods):
            if m not in exported:
                rel = os.path.relpath(route, REPO_ROOT)
                errors.append(f"{url}: documents {m} but {rel} does not export it "
                              f"(exports: {sorted(exported) or 'none'})")
    if errors:
        print("OpenAPI spec documents routes/methods that don't exist:", file=sys.stderr)
        for e in errors:
            print(f"  ✗ {e}", file=sys.stderr)
        print("\nFix api/openapi/openapi.yaml to match the real dashboard routes "
              "(dashboard/src/app/api/**/route.ts).", file=sys.stderr)
        return 1
    print(f"✓ openapi-routes: {len(spec_paths)} documented paths all map to real "
          "route handlers with matching methods")
    return 0


if __name__ == "__main__":
    sys.exit(main())
