import { describe, expect, test } from 'bun:test'

import { groupMatchesSupplier } from './bindings'

const bindings = [
  { group_id: 2, supplier_id: 10, profile_id: 100 },
  { group_id: 3, supplier_id: 10, profile_id: 101 },
  { group_id: 4, supplier_id: 11, profile_id: 110 },
]

describe('supplier binding choices', () => {
  test('offers only groups with the identical supplier and profile', () => {
    const selected = { supplier_id: 10, profile_id: 100 }
    const groups = [1, 2, 3, 4].filter((id) =>
      groupMatchesSupplier(id, bindings, selected)
    )
    expect(groups).toEqual([2])
  })

  test('unbound channels can choose only default pricing groups', () => {
    const groups = [1, 2, 3, 4].filter((id) =>
      groupMatchesSupplier(id, bindings, {
        supplier_id: null,
        profile_id: null,
      })
    )
    expect(groups).toEqual([1])
  })

  test('supplier selection without a profile never matches a bound group', () => {
    expect(
      groupMatchesSupplier(2, bindings, { supplier_id: 10, profile_id: null })
    ).toBe(false)
  })
})
