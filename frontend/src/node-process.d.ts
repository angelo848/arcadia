// Minimal typing of the Node `process` that sessionForm.test.ts uses to pin the time zone (no @types/node here).
declare const process: { env: Record<string, string | undefined> }
