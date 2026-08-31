import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  cryptoAmountTailVariants,
  decimalUsdtToMicroUnits,
  CRYPTO_AMOUNT_TAIL_DEFAULT_UNITS,
  CRYPTO_NETWORKS,
  CRYPTO_PAYMENT_CURRENCY,
  CRYPTO_RECEIVING_ADDRESS_FIELDS,
  isCryptoNetworkPayable,
  legacyPrecisionToTailLimitUnits,
  cryptoPrecisionToTailLimitUnits,
  cryptoTailLimitUnitsToPrecision,
  microUnitsToDecimalUsdt,
  resolveCryptoTailLimitSettings,
  shouldUpdateCryptoPaymentCredential,
} from './crypto-payment-settings'

describe('crypto payment settings', () => {
  test('preserves a masked TronGrid API key unless it is replaced', () => {
    assert.equal(shouldUpdateCryptoPaymentCredential('', '********'), false)
    assert.equal(
      shouldUpdateCryptoPaymentCredential('********', '********'),
      false
    )
    assert.equal(
      shouldUpdateCryptoPaymentCredential('new-key', '********'),
      true
    )
  })

  test('fixes direct crypto currency to USDT', () => {
    assert.equal(CRYPTO_PAYMENT_CURRENCY, 'USDT')
  })

  test('keeps one receiving address field per configured network', () => {
    assert.deepEqual(
      CRYPTO_RECEIVING_ADDRESS_FIELDS.map(({ name, network }) => [
        name,
        network,
      ]),
      [
        ['USDTTRC20ReceivingAddress', 'TRON'],
        ['USDTTONReceivingAddress', 'TON'],
        ['USDTSolanaReceivingAddress', 'SOLANA'],
        ['USDTSolanaReceivingTokenAccount', 'SOLANA'],
      ]
    )
  })

  test('advertises supported direct payment networks', () => {
    assert.deepEqual(
      CRYPTO_NETWORKS.filter((option) => option.supported).map(
        (option) => option.value
      ),
      ['TRON', 'TON', 'SOLANA']
    )
    assert.equal(isCryptoNetworkPayable('TRON'), true)
    assert.equal(isCryptoNetworkPayable('TON'), true)
    assert.equal(isCryptoNetworkPayable('Solana'), true)
  })

  test('converts decimal USDT limits using exact micro-units', () => {
    assert.equal(CRYPTO_AMOUNT_TAIL_DEFAULT_UNITS, 10_000)
    assert.equal(decimalUsdtToMicroUnits('0.000002'), 2)
    assert.equal(microUnitsToDecimalUsdt(2), '0.000002')
    assert.equal(decimalUsdtToMicroUnits('0.001'), 1_000)
    assert.equal(decimalUsdtToMicroUnits('0.010000'), 10_000)
    assert.equal(decimalUsdtToMicroUnits('0.000001'), null)
    assert.equal(decimalUsdtToMicroUnits('0.010001'), null)
    assert.equal(decimalUsdtToMicroUnits('1e-3'), null)
    assert.equal(microUnitsToDecimalUsdt(10_000), '0.01')
    assert.equal(microUnitsToDecimalUsdt(1_000), '0.001')
    assert.equal(cryptoAmountTailVariants('0.001'), 999)
    assert.equal(cryptoAmountTailVariants('0.01'), 9_999)
    assert.equal(cryptoAmountTailVariants('0.000001'), null)
  })

  test('maps legacy precision only when the new setting is absent', () => {
    assert.equal(legacyPrecisionToTailLimitUnits('3'), 10)
    assert.equal(legacyPrecisionToTailLimitUnits(6), 10_000)
    assert.equal(legacyPrecisionToTailLimitUnits('7'), null)

    const defaults = { USDTTRC20AmountTailLimitUnits: 10_000 }
    assert.deepEqual(
      resolveCryptoTailLimitSettings(defaults, [
        { key: 'USDTTRC20AmountPrecision', value: '4' },
      ]),
      { USDTTRC20AmountTailLimitUnits: 100 }
    )
    assert.deepEqual(
      resolveCryptoTailLimitSettings(defaults, [
        { key: 'USDTTRC20AmountTailLimitUnits', value: '1000' },
        { key: 'USDTTRC20AmountPrecision', value: '4' },
      ]),
      defaults
    )
  })

  test('maps the precision selector to stable tail limits', () => {
    assert.equal(cryptoPrecisionToTailLimitUnits('3'), 10)
    assert.equal(cryptoPrecisionToTailLimitUnits(6), 10_000)
    assert.equal(cryptoTailLimitUnitsToPrecision(100), 4)
    assert.equal(cryptoTailLimitUnitsToPrecision(50), null)
  })
})
