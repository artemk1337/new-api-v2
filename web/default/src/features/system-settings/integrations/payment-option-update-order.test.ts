import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { orderPaymentOptionUpdates } from './payment-option-update-order'

describe('payment option update order', () => {
  test('saves gateway prerequisites before readiness fields', () => {
    const updates = orderPaymentOptionUpdates([
      { key: 'WaffoEnabled' },
      { key: 'CreemWebhookSecret' },
      { key: 'WaffoApiKey' },
      { key: 'CreemTestMode' },
      { key: 'CreemProducts' },
      { key: 'WaffoCurrency' },
      { key: 'CreemApiKey' },
      { key: 'WaffoPublicCert' },
      { key: 'YooKassaEnabled' },
      { key: 'YooKassaShopID' },
      { key: 'YooKassaSecretKey' },
      { key: 'YooKassaReturnURL' },
      { key: 'NOWPaymentsEnabled' },
      { key: 'NOWPaymentsAPIKey' },
      { key: 'NOWPaymentsIPNSecret' },
      { key: 'NOWPaymentsIPNCallbackURL' },
    ])
    const keys = updates.map((update) => update.key)

    assert.ok(keys.indexOf('WaffoCurrency') < keys.indexOf('WaffoApiKey'))
    assert.ok(keys.indexOf('WaffoCurrency') < keys.indexOf('WaffoPublicCert'))
    assert.ok(keys.indexOf('WaffoCurrency') < keys.indexOf('WaffoEnabled'))
    assert.ok(keys.indexOf('WaffoApiKey') < keys.indexOf('WaffoEnabled'))
    assert.ok(keys.indexOf('WaffoPublicCert') < keys.indexOf('WaffoEnabled'))
    assert.ok(keys.indexOf('CreemProducts') < keys.indexOf('CreemApiKey'))
    assert.ok(keys.indexOf('CreemApiKey') < keys.indexOf('CreemWebhookSecret'))
    assert.ok(keys.indexOf('CreemProducts') < keys.indexOf('CreemWebhookSecret'))
    assert.ok(keys.indexOf('CreemProducts') < keys.indexOf('CreemTestMode'))
    assert.ok(keys.indexOf('YooKassaShopID') < keys.indexOf('YooKassaEnabled'))
    assert.ok(keys.indexOf('YooKassaSecretKey') < keys.indexOf('YooKassaEnabled'))
    assert.ok(keys.indexOf('YooKassaReturnURL') < keys.indexOf('YooKassaEnabled'))
    assert.ok(keys.indexOf('NOWPaymentsAPIKey') < keys.indexOf('NOWPaymentsEnabled'))
    assert.ok(keys.indexOf('NOWPaymentsIPNSecret') < keys.indexOf('NOWPaymentsEnabled'))
    assert.ok(keys.indexOf('NOWPaymentsIPNCallbackURL') < keys.indexOf('NOWPaymentsEnabled'))
  })

  test('keeps the original order when prerequisites are absent', () => {
    const keys = orderPaymentOptionUpdates([
      { key: 'PayAddress' },
      { key: 'StripeApiSecret' },
      { key: 'NOWPaymentsEnabled' },
    ]).map((update) => update.key)

    assert.deepEqual(keys, ['PayAddress', 'StripeApiSecret', 'NOWPaymentsEnabled'])
  })
})
