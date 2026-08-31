import assert from 'node:assert/strict'
import { test } from 'node:test'

import { shouldUpdateCryptoProviderCredential } from './crypto-provider-credentials'

test('updates a non-empty replacement while preserving masked credentials', () => {
  assert.equal(
    shouldUpdateCryptoProviderCredential('new-token', '********'),
    true
  )
  assert.equal(shouldUpdateCryptoProviderCredential('', '********'), false)
  assert.equal(
    shouldUpdateCryptoProviderCredential('********', '********'),
    false
  )
})
