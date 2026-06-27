#!/usr/bin/env node
/**
 * Generates TypeScript types from Kubernetes CRD OpenAPI schemas.
 *
 * This script reads CRD YAML files from config/crd/bases/ and generates
 * corresponding TypeScript interfaces in dashboard/src/types/generated/.
 *
 * Usage: node scripts/generate-dashboard-types.js
 */

const fs = require('fs');
const path = require('path');

// Use js-yaml from dashboard's node_modules
const yaml = require(path.join(__dirname, '..', 'dashboard', 'node_modules', 'js-yaml'));

const CRD_DIR = path.join(__dirname, '..', 'config', 'crd', 'bases');
const OUTPUT_DIR = path.join(__dirname, '..', 'dashboard', 'src', 'types', 'generated');

// Map of CRD files to generate
const CRDS = [
  { file: 'omnia.altairalabs.ai_agentruntimes.yaml', name: 'AgentRuntime' },
  { file: 'omnia.altairalabs.ai_promptpacks.yaml', name: 'PromptPack' },
  { file: 'omnia.altairalabs.ai_toolregistries.yaml', name: 'ToolRegistry' },
  { file: 'omnia.altairalabs.ai_providers.yaml', name: 'Provider' },
  { file: 'omnia.altairalabs.ai_sessionretentionpolicies.yaml', name: 'SessionRetentionPolicy' },
  { file: 'omnia.altairalabs.ai_skillsources.yaml', name: 'SkillSource' },
];

// Convert OpenAPI type to TypeScript type
function openApiTypeToTs(schema, indent = '') {
  if (!schema) return 'unknown';

  // Handle $ref (not common in CRDs but just in case)
  if (schema.$ref) {
    const refName = schema.$ref.split('/').pop();
    return refName;
  }

  // Handle enums
  if (schema.enum) {
    return schema.enum.map(v => `"${v}"`).join(' | ');
  }

  // Handle basic types
  switch (schema.type) {
    case 'string':
      return 'string';
    case 'integer':
    case 'number':
      return 'number';
    case 'boolean':
      return 'boolean';
    case 'array':
      const itemType = openApiTypeToTs(schema.items, indent);
      // Wrap top-level union types in parens for correct precedence: ("a" | "b")[] not "a" | "b"[]
      // Object types ({...}) don't need wrapping even if they contain unions internally
      if (itemType.includes(' | ') && !itemType.startsWith('{')) {
        return `(${itemType})[]`;
      }
      return `${itemType}[]`;
    case 'object':
      // Handle additionalProperties (maps)
      if (schema.additionalProperties) {
        const valueType = openApiTypeToTs(schema.additionalProperties, indent);
        return `Record<string, ${valueType}>`;
      }
      // Handle properties
      if (schema.properties) {
        return generateInterface(schema, indent);
      }
      return 'Record<string, unknown>';
    default:
      // No type specified, check for properties
      if (schema.properties) {
        return generateInterface(schema, indent);
      }
      return 'unknown';
  }
}

// Generate TypeScript interface from OpenAPI schema
function generateInterface(schema, indent = '') {
  if (!schema.properties) return '{}';

  const required = new Set(schema.required || []);
  const lines = ['{'];
  const nextIndent = indent + '  ';

  for (const [propName, propSchema] of Object.entries(schema.properties)) {
    const isRequired = required.has(propName);
    const tsType = openApiTypeToTs(propSchema, nextIndent);
    const optional = isRequired ? '' : '?';

    // Add description as JSDoc comment
    if (propSchema.description) {
      const desc = propSchema.description.replace(/\n/g, `\n${nextIndent} * `);
      lines.push(`${nextIndent}/** ${desc} */`);
    }

    lines.push(`${nextIndent}${propName}${optional}: ${tsType};`);
  }

  lines.push(`${indent}}`);
  return lines.join('\n');
}

// Extract and generate types for a single CRD
function processCrd(crdPath, resourceName) {
  const content = fs.readFileSync(crdPath, 'utf8');
  const crd = yaml.load(content);

  // Get the schema from the first version
  const version = crd.spec.versions[0];
  const schema = version.schema.openAPIV3Schema;

  if (!schema || !schema.properties) {
    console.warn(`  Warning: No schema found in ${crdPath}`);
    return null;
  }

  const lines = [
    `// Auto-generated from ${path.basename(crdPath)}`,
    `// Do not edit manually - run 'make generate-dashboard-types' to regenerate`,
    '',
    `import type { ObjectMeta } from "../common";`,
    '',
  ];

  // Generate spec type
  if (schema.properties.spec) {
    const specSchema = schema.properties.spec;
    lines.push(`export interface ${resourceName}Spec ${generateInterface(specSchema)}`);
    lines.push('');
  }

  // Generate status type
  if (schema.properties.status) {
    const statusSchema = schema.properties.status;
    lines.push(`export interface ${resourceName}Status ${generateInterface(statusSchema)}`);
    lines.push('');
  }

  // Generate main resource type
  const group = crd.spec.group;
  const versionName = version.name;
  lines.push(`export interface ${resourceName} {`);
  lines.push(`  apiVersion: "${group}/${versionName}";`);
  lines.push(`  kind: "${resourceName}";`);
  lines.push(`  metadata: ObjectMeta;`);
  if (schema.properties.spec) {
    lines.push(`  spec: ${resourceName}Spec;`);
  }
  if (schema.properties.status) {
    lines.push(`  status?: ${resourceName}Status;`);
  }
  lines.push('}');
  lines.push('');

  return lines.join('\n');
}

// --- Constraint extraction (issue #1612) ---

const CONSTRAINT_KEYS = ['pattern', 'enum', 'minLength', 'maxLength', 'minimum', 'maximum'];

// Walk a schema node, collecting field-path -> constraint entries.
// Array items append "[]" to the path; required comes from each object's
// `required` array. x-kubernetes-validations (CEL) is skipped and logged.
function collectConstraints(schema, prefix, out, requiredFields) {
  if (!schema || typeof schema !== 'object') return;

  if (prefix && schema['x-kubernetes-validations']) {
    console.log(`    (skipping CEL rule on ${prefix})`);
  }

  const constraint = {};
  if (schema.type && schema.type !== 'object' && schema.type !== 'array') {
    constraint.type = schema.type;
  }
  for (const key of CONSTRAINT_KEYS) {
    if (schema[key] !== undefined) constraint[key] = schema[key];
  }
  if (prefix && requiredFields && requiredFields.has(lastSegment(prefix))) {
    constraint.required = true;
  }
  if (prefix && Object.keys(constraint).length > 0) {
    out[prefix] = constraint;
  }

  if (schema.properties) {
    const req = new Set(schema.required || []);
    for (const [name, child] of Object.entries(schema.properties)) {
      const childPrefix = prefix ? `${prefix}.${name}` : name;
      collectConstraints(child, childPrefix, out, req);
    }
  }
  if (schema.type === 'array' && schema.items) {
    collectConstraints(schema.items, `${prefix}[]`, out, null);
  }
}

function lastSegment(prefix) {
  return prefix.split('.').pop();
}

// Build the constraint map for one CRD's spec schema.
function extractConstraints(crdSchema) {
  const out = {};
  if (crdSchema && crdSchema.properties && crdSchema.properties.spec) {
    collectConstraints(crdSchema.properties.spec, 'spec', out, new Set());
  }
  return out;
}

// Main execution
function main() {
  console.log('Generating TypeScript types from CRDs...\n');

  // Ensure output directory exists
  if (!fs.existsSync(OUTPUT_DIR)) {
    fs.mkdirSync(OUTPUT_DIR, { recursive: true });
  }

  const indexExports = [
    '// Auto-generated index file',
    '// Do not edit manually - run \'make generate-dashboard-types\' to regenerate',
    '',
  ];

  const allConstraints = {};

  for (const { file, name } of CRDS) {
    const crdPath = path.join(CRD_DIR, file);

    if (!fs.existsSync(crdPath)) {
      console.log(`  Skipping ${file} (not found)`);
      continue;
    }

    console.log(`  Processing ${file}...`);
    const content = processCrd(crdPath, name);

    if (content) {
      const outputFile = `${name.toLowerCase()}.ts`;
      const outputPath = path.join(OUTPUT_DIR, outputFile);
      fs.writeFileSync(outputPath, content);
      console.log(`    -> ${outputFile}`);

      indexExports.push(`export * from "./${name.toLowerCase()}";`);
    }

    const crdYaml = yaml.load(fs.readFileSync(crdPath, 'utf8'));
    const crdSchema = crdYaml.spec.versions[0].schema.openAPIV3Schema;
    allConstraints[name] = extractConstraints(crdSchema);
  }

  // Write index file
  indexExports.push('');
  fs.writeFileSync(path.join(OUTPUT_DIR, 'index.ts'), indexExports.join('\n'));
  console.log(`    -> index.ts`);

  const constraintsHeader = [
    '// Auto-generated from CRD OpenAPI schemas (issue #1612).',
    "// Do not edit manually - run 'make generate-dashboard-types' to regenerate.",
    '',
    'import type { FieldConstraint } from "@/lib/validation/constraint-types";',
    '',
    'export const crdConstraints: Record<string, Record<string, FieldConstraint>> =',
    `  ${JSON.stringify(allConstraints, null, 2)};`,
    '',
  ].join('\n');
  fs.writeFileSync(path.join(OUTPUT_DIR, 'crd-constraints.ts'), constraintsHeader);
  console.log('    -> crd-constraints.ts');

  console.log('\nDone!');
  console.log(`\nNote: Generated types are in ${path.relative(process.cwd(), OUTPUT_DIR)}/`);
  console.log('The hand-written types in dashboard/src/types/ are still the source of truth.');
  console.log('Use these generated types as a reference or merge them manually.');
}

main();
