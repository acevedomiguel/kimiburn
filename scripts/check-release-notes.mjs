#!/usr/bin/env node
// Render fixture notes in CI so dependency drift cannot fail only at release.
import { generateNotes } from "@semantic-release/release-notes-generator";

const commits = [
  { hash: "0123456789abcdef0123456789abcdef01234567", message: "feat: add the dashboard\n\nA longer body." },
  { hash: "89abcdef0123456789abcdef0123456789abcdef", message: "fix: correct the timezone" },
];

const notes = await generateNotes(
  { preset: "conventionalcommits" },
  {
    options: { repositoryUrl: "https://github.com/acevedomiguel/kimiburn.git" },
    lastRelease: {},
    nextRelease: { version: "1.0.0", gitTag: "v1.0.0", gitHead: commits[0].hash },
    commits,
    logger: { log: () => {} },
    cwd: process.cwd(),
  },
);

const failures = [];
if (!notes.includes("### Features")) failures.push("the Features section is missing");
if (!notes.includes("### Bug Fixes")) failures.push("the Bug Fixes section is missing");
if (!notes.includes("add the dashboard")) failures.push("the feature summary is missing");
if (!notes.includes("correct the timezone")) failures.push("the fix summary is missing");
if (notes.includes("Missing helper")) failures.push("the writer guard fired");
if (failures.length > 0) {
  console.error("Release notes could not be rendered:", failures, notes);
  process.exit(1);
}
console.log("Release notes render correctly.");
