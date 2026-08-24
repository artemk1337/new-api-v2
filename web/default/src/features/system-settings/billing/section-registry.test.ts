import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getBillingSectionRedirect } from './section-redirect'
import { BILLING_SECTION_IDS } from './section-registry'

describe('billing section registry', () => {
  test('redirects retired currency pages to platform currencies', () => {
    assert.equal(getBillingSectionRedirect('currency'), 'platform-currencies')
    assert.equal(
      getBillingSectionRedirect('currency-exchange-rate'),
      'platform-currencies'
    )
  })

  test('does not render a redundant currency navigation section', () => {
    assert.equal(
      (BILLING_SECTION_IDS as readonly string[]).includes('currency'),
      false
    )
  })
})
