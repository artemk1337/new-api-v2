import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  canRenderStructuredPricing,
  parseTiersFromExpr,
} from './billing-expr.ts'
import {
  evalExprLocally,
  generateExprFromVisualConfig,
  tryParseVisualConfig,
} from './tier-expr.ts'

test('renders and preserves reasoning-output pricing in tiered expressions', () => {
  const expression = 'tier("base", p * 0.8 + c * 2 + rt * 8)'

  const tiers = parseTiersFromExpr(expression)
  assert.equal(tiers.length, 1)
  assert.equal(tiers[0].reasoningOutputPrice, 8)

  const visualConfig = tryParseVisualConfig(expression)
  assert.ok(visualConfig)
  assert.equal(visualConfig.tiers[0].reasoning_output_unit_cost, 8)
  assert.match(generateExprFromVisualConfig(visualConfig), /rt \* 8/)
})

test('renders parsed tiers without a request matched tier', () => {
  const tiers = parseTiersFromExpr('tier("step_1", p * 4 + c * 16)')

  assert.equal(canRenderStructuredPricing(tiers), true)
})

test('falls back when a request matched tier is unknown', () => {
  const tiers = parseTiersFromExpr('tier("step_1", p * 4 + c * 16)')

  assert.equal(canRenderStructuredPricing(tiers, 'missing', true), false)
})

test('falls back when usage details have no matched tier', () => {
  const tiers = parseTiersFromExpr('tier("step_1", p * 4 + c * 16)')

  assert.equal(canRenderStructuredPricing(tiers, undefined, true), false)
})

test('renders when a request matched tier has normalized label', () => {
  const tiers = parseTiersFromExpr('tier("Step 1", p * 4 + c * 16)')

  assert.equal(canRenderStructuredPricing(tiers, ' step1 ', true), true)
})

test('opens a synchronized reasoning contract in the visual editor', () => {
  const expression =
    'tier("step_1", p * 0.8 + c * 2 + cr * 0.1 + cc * 0.2 + cc1h * 0.4 + ai * 0 + ao * 0 + rt * 8)'

  const visualConfig = tryParseVisualConfig(expression)
  assert.ok(visualConfig)
  assert.equal(visualConfig.tiers[0].reasoning_output_unit_cost, 8)
  assert.match(generateExprFromVisualConfig(visualConfig), /rt \* 8/)

  visualConfig.tiers[0].reasoning_output_unit_cost = 0
  assert.doesNotMatch(generateExprFromVisualConfig(visualConfig), /rt \* 0/)
})

test('opens multi-tier contracts with explicit zero lanes', () => {
  const expression =
    'len <= 128000 ? tier("step_1", p * 0.8 + c * 2 + cr * 0 + cc * 0 + cc1h * 0 + ai * 0 + ao * 0 + rt * 8) : tier("step_2", p * 1.6 + c * 4 + cr * 0 + cc * 0 + cc1h * 0 + ai * 0 + ao * 0 + rt * 12)'

  const visualConfig = tryParseVisualConfig(expression)
  assert.ok(visualConfig)
  assert.equal(visualConfig.tiers.length, 2)
  assert.equal(visualConfig.tiers[1].reasoning_output_unit_cost, 12)
  assert.match(generateExprFromVisualConfig(visualConfig), /cr \* 0/)
  assert.match(generateExprFromVisualConfig(visualConfig), /ao \* 0/)
})

test('rejects tier fragments embedded in unsupported expressions', () => {
  assert.equal(tryParseVisualConfig('tier("base", p * 1 + c * 2) + p'), null)
})

test('estimates reasoning tokens as a subset of output tokens', () => {
  const result = evalExprLocally('tier("base", c * 2 + rt * 8)', 0, 100, {
    cacheReadTokens: 0,
    cacheCreateTokens: 0,
    cacheCreate1hTokens: 0,
    imageTokens: 0,
    imageOutputTokens: 0,
    audioInputTokens: 0,
    audioOutputTokens: 0,
    reasoningOutputTokens: 40,
  })

  assert.equal(result.cost, 440)
})
