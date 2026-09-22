// ESLint for the console and device page scripts (internal/web/static).
//
// They are classic <script> files sharing one global scope per page, so
// cross-file names are checked by the type checker (tsconfig.*.json), not
// by no-undef. max-lines-per-function and max-depth carry CLAUDE.md's KISS
// guardrail (≤30 lines, ≤3 levels) over to the JavaScript.
import js from "@eslint/js";
import globals from "globals";

export default [
  js.configs.recommended,
  {
    files: ["internal/web/static/**/*.js"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "script",
      globals: { ...globals.browser },
    },
    rules: {
      "no-undef": "off",
      // Best-effort calls (aborted fetches, pointer capture, decoder close)
      // swallow their errors on purpose.
      "no-empty": ["error", { allowEmptyCatch: true }],
      // A top-level const may shadow a legacy window property (status).
      "no-redeclare": ["error", { builtinGlobals: false }],
      // Top-level declarations are the page's shared globals, used by the
      // other scripts on the page, so only locals are checked.
      "no-unused-vars": ["error", { vars: "local", args: "after-used", caughtErrors: "none" }],
      // Warn until the console scripts are split (QUALITY-PLAN item 12),
      // then raise to error.
      "max-lines-per-function": ["warn", { max: 30, skipBlankLines: true, skipComments: true }],
      "max-depth": ["warn", 3],
    },
  },
];
