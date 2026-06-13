/* eslint-disable sonarjs/slow-regex -- Monarch tokenizer rules are applied by
   Monaco per editor line (short, bounded input), not against untrusted unbounded
   strings, so the slow-regex/ReDoS concern doesn't apply to a grammar file. */
import type { languages } from "monaco-editor";

// The @codingame editor-api (the monaco-editor alias) ships no basic-languages,
// so there's no YAML tokenizer and the editor renders plain/uncoloured. Register
// a pragmatic monarch grammar to restore syntax colouring. (The full VS Code
// TextMate grammar would come from @codingame/monaco-vscode-yaml-default-extension
// plus the textmate/theme service overrides — a heavier follow-up; this covers
// the common YAML constructs and PromptKit's {{template}} variables.)
export const yamlMonarchLanguage: languages.IMonarchLanguage = {
  tokenPostfix: ".yaml",
  brackets: [
    { token: "delimiter.bracket", open: "{", close: "}" },
    { token: "delimiter.square", open: "[", close: "]" },
  ],
  tokenizer: {
    root: [
      [/#.*$/, "comment"],
      // key:
      [/([^\s:#][^:#]*?)(\s*)(:)(?=\s|$)/, ["type", "white", "delimiter"]],
      // block sequence dash
      [/^\s*-(?=\s|$)/, "delimiter"],
      // anchors / aliases / tags
      [/[&*][\w-]+/, "namespace"],
      [/![\w/-]+/, "tag"],
      // strings
      [/"/, "string", "@string_double"],
      [/'/, "string", "@string_single"],
      // PromptKit template variables
      [/\{\{[^}]*\}\}/, "variable"],
      // numbers
      [/[-+]?\d+\.\d*(?:[eE][-+]?\d+)?/, "number.float"],
      [/[-+]?\d+(?:[eE][-+]?\d+)?/, "number"],
      // booleans / null
      [/\b(?:true|True|TRUE|false|False|FALSE|null|Null|NULL)\b/, "keyword"],
      // flow brackets
      [/[{}[\],]/, "@brackets"],
    ],
    string_double: [
      [/\{\{[^}]*\}\}/, "variable"],
      [/[^\\"{]+/, "string"],
      [/\\./, "string.escape"],
      [/"/, "string", "@pop"],
    ],
    string_single: [
      [/\{\{[^}]*\}\}/, "variable"],
      [/[^\\'{]+/, "string"],
      [/\\./, "string.escape"],
      [/'/, "string", "@pop"],
    ],
  },
};
