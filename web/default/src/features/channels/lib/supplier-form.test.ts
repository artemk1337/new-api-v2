import { describe, expect, test } from 'bun:test'

import { channelSchema } from '../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from './channel-form'
import { aggregateChannelsBySupplier } from './channel-utils'

function channel(
  id: number,
  supplierId: number | null,
  profileId: number | null
) {
  return channelSchema.parse({
    id,
    supplier_id: supplierId,
    supplier_profile_id: profileId,
    type: 1,
    key: 'private-upstream-key',
    status: 1,
    name: 'Channel',
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'model',
    group: '2',
    tag: 'legacy-name',
  })
}

describe('channel supplier bindings', () => {
  test('preserves an existing binding while editing other channel fields', () => {
    const values = transformChannelToFormDefaults(channel(1, 10, 100))
    const payload = transformFormDataToUpdatePayload(
      { ...values, name: 'Renamed' },
      1
    )
    expect(payload.supplier_id).toBe(10)
    expect(payload.supplier_profile_id).toBe(100)
    expect(payload.key).toBeUndefined()
    expect(payload.tag).toBe('legacy-name')
  })

  test('sends explicit nulls to remove a binding and uses default pricing for new channels', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'New channel',
      models: 'model',
      group: ['1'],
      key: 'key',
    }
    expect(transformFormDataToCreatePayload(values).channel).toMatchObject({
      supplier_id: null,
      supplier_profile_id: null,
    })
    expect(transformFormDataToUpdatePayload(values, 1)).toMatchObject({
      supplier_id: null,
      supplier_profile_id: null,
    })
  })

  test('rejects a supplier/profile pair with either side missing', () => {
    const values = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'New channel',
      models: 'model',
      group: ['1'],
      key: 'key',
    }
    expect(
      channelFormSchema.safeParse({
        ...values,
        supplier_id: 10,
        supplier_profile_id: null,
      }).success
    ).toBe(false)
    expect(
      channelFormSchema.safeParse({
        ...values,
        supplier_id: null,
        supplier_profile_id: 100,
      }).success
    ).toBe(false)
    expect(
      channelFormSchema.safeParse({
        ...values,
        supplier_id: 10,
        supplier_profile_id: 100,
      }).success
    ).toBe(true)
  })

  test('groups by stable supplier IDs rather than legacy tags or duplicate supplier names', () => {
    const channels = [
      channel(1, 10, 100),
      channel(2, 11, 110),
      channel(3, 10, 101),
      channel(4, null, null),
    ]
    const rows = aggregateChannelsBySupplier(
      channels,
      new Map([
        [10, 'Provider'],
        [11, 'Provider'],
      ])
    )
    expect(rows.map((row) => row.children.map((child) => child.id))).toEqual([
      [1, 3],
      [2],
      [4],
    ])
    expect(new Set(rows.map((row) => row.id)).size).toBe(3)
    expect(rows.every((row) => row.tag === null)).toBe(true)
  })
})
