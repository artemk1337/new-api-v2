import React, { useState, useCallback, useMemo } from 'react';
import { Button, Select, Typography, Popconfirm } from '@douyinfe/semi-ui';
import { IconPlus, IconDelete } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

let _idCounter = 0;
const uid = () => `ag_${++_idCounter}`;

function parseAutoGroups(str) {
  if (!str || !str.trim()) return [];
  try {
    const parsed = JSON.parse(str);
    if (!Array.isArray(parsed)) return [];
    const seen = new Set();
    return parsed.flatMap((item) => {
      if (typeof item !== 'string') return [];
      const name = item.trim();
      if (!name || seen.has(name)) return [];
      seen.add(name);
      return [{ _id: uid(), name }];
    });
  } catch {
    return [];
  }
}

function serializeAutoGroups(items) {
  const names = [...new Set(items.map((i) => i.name.trim()).filter(Boolean))];
  return names.length === 0 ? '' : JSON.stringify(names);
}

export default function AutoGroupList({ value, groupNames = [], onChange }) {
  const { t } = useTranslation();

  const [items, setItems] = useState(() => parseAutoGroups(value));

  const emitChange = useCallback(
    (newItems) => {
      setItems(newItems);
      onChange?.(serializeAutoGroups(newItems));
    },
    [onChange],
  );

  const groupOptions = useMemo(
    () =>
      groupNames.map((group) =>
        typeof group === 'string' ? { value: group, label: group } : group,
      ),
    [groupNames],
  );

  const selectedGroups = useMemo(
    () => new Set(items.map((item) => item.name).filter(Boolean)),
    [items],
  );
  const canAddItem =
    !items.some((item) => !item.name) &&
    groupOptions.some((option) => !selectedGroups.has(option.value));

  const addItem = useCallback(() => {
    if (!canAddItem) return;
    emitChange([...items, { _id: uid(), name: '' }]);
  }, [canAddItem, items, emitChange]);

  const removeItem = useCallback(
    (id) => {
      if (items.length <= 1) return;
      emitChange(items.filter((i) => i._id !== id));
    },
    [items, emitChange],
  );

  const updateItem = useCallback(
    (id, name) => {
      if (items.some((item) => item._id !== id && item.name === name)) return;
      emitChange(items.map((i) => (i._id === id ? { ...i, name } : i)));
    },
    [items, emitChange],
  );

  if (items.length === 0) {
    return (
      <div>
        <Text type='tertiary' className='block text-center py-4'>
          {t('暂无自动分组，点击下方按钮添加')}
        </Text>
        <div className='mt-2 flex justify-center'>
          <Button
            icon={<IconPlus />}
            theme='outline'
            onClick={addItem}
            disabled={!canAddItem}
          >
            {t('添加分组')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className='space-y-2'>
        {items.map((item) => (
          <div
            key={item._id}
            className='flex items-center gap-2'
          >
            <Select
              size='small'
              filter
              value={item.name || undefined}
              placeholder={t('选择分组')}
              optionList={groupOptions.filter(
                (option) =>
                  option.value === item.name ||
                  !selectedGroups.has(option.value),
              )}
              onChange={(v) => updateItem(item._id, v)}
              style={{ flex: 1 }}
              position='bottomLeft'
            />
            <Popconfirm
              title={t('确认移除？')}
              onConfirm={() => removeItem(item._id)}
              position='left'
            >
              <Button
                icon={<IconDelete />}
                type='danger'
                theme='borderless'
                size='small'
                disabled={items.length <= 1}
              />
            </Popconfirm>
          </div>
        ))}
      </div>
      <div className='mt-3 flex justify-center'>
        <Button
          icon={<IconPlus />}
          theme='outline'
          onClick={addItem}
          disabled={!canAddItem}
        >
          {t('添加分组')}
        </Button>
      </div>
    </div>
  );
}
