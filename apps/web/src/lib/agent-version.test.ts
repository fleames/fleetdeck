import { test } from "node:test";
import assert from "node:assert/strict";
import {
  agentVersionOutdated,
  agentSupportsPanelUpdate,
} from "./agent-version.ts";

test("agentVersionOutdated: equal and empty", () => {
  assert.equal(agentVersionOutdated("0.4.3-dev", "0.4.3-dev"), false);
  assert.equal(agentVersionOutdated(null, "0.4.3-dev"), false);
  assert.equal(agentVersionOutdated("0.4.3-dev", null), false);
  assert.equal(agentVersionOutdated("", "0.4.3-dev"), false);
});

test("agentVersionOutdated: older agent vs panel expected", () => {
  assert.equal(agentVersionOutdated("0.4.2-dev", "0.4.3-dev"), true);
  assert.equal(agentVersionOutdated("0.4.0", "0.4.3-dev"), true);
  assert.equal(agentVersionOutdated("0.3.9-dev", "0.4.3-dev"), true);
});

test("agentVersionOutdated: agent newer than stale panel expected is not outdated", () => {
  // Regression: panel still on AGENT_VERSION=0.4.2-dev after publishing 0.4.3-dev
  assert.equal(agentVersionOutdated("0.4.3-dev", "0.4.2-dev"), false);
  assert.equal(agentVersionOutdated("0.5.0-dev", "0.4.3-dev"), false);
});

test("agentVersionOutdated: -dev suffix does not change numeric compare", () => {
  assert.equal(agentVersionOutdated("0.4.3-dev", "0.4.3"), false);
  assert.equal(agentVersionOutdated("0.4.2-dev", "0.4.3"), true);
});

test("agentSupportsPanelUpdate", () => {
  assert.equal(agentSupportsPanelUpdate("0.4.2-dev"), true);
  assert.equal(agentSupportsPanelUpdate("0.4.3-dev"), true);
  assert.equal(agentSupportsPanelUpdate("0.4.1"), false);
  assert.equal(agentSupportsPanelUpdate(null), false);
});
