/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useState, useCallback, useMemo, useEffect } from 'react';
import {
  Button,
  Table,
  Tag,
  Empty,
  Checkbox,
  Form,
  Input,
  Tooltip,
  Select,
  Modal,
  Spin,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { RefreshCcw, CheckSquare, AlertTriangle } from 'lucide-react';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  showWarning,
  stringToColor,
} from '../../../helpers';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { useTranslation } from 'react-i18next';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
const PRICING_ENDPOINT = '/api/pricing';
const RATIO_CONFIG_ENDPOINT = '/api/ratio_config';
const OPENROUTER_ENDPOINT = 'openrouter';
const OFFICIAL_CHANNEL_ID = -100;
const MODELS_DEV_PRESET_ID = -101;
const OFFICIAL_CHANNEL_ENDPOINT =
  'https://basellm.github.io/llm-metadata/api/newapi/ratio_config-v1-base.json';
const MODELS_DEV_PRESET_ENDPOINT = 'https://models.dev/api.json';
const OPENROUTER_CHANNEL_TYPE = 20;

const syncIntervals = [
  { value: 0, label: 'Не обновлять автоматически' },
  { value: 60, label: '1 минута' },
  { value: 600, label: '10 минут' },
  { value: 1800, label: '30 минут' },
  { value: 3600, label: '1 час' },
];

function defaultEndpointForChannel(channel) {
  if (channel.id === OFFICIAL_CHANNEL_ID) return OFFICIAL_CHANNEL_ENDPOINT;
  if (channel.id === MODELS_DEV_PRESET_ID) return MODELS_DEV_PRESET_ENDPOINT;
  if (channel.type === OPENROUTER_CHANNEL_TYPE) return OPENROUTER_ENDPOINT;
  return PRICING_ENDPOINT;
}

function ConflictConfirmModal({ t, visible, items, loading, onOk, onCancel }) {
  const isMobile = useIsMobile();
  const columns = [
    { title: t('渠道'), dataIndex: 'channel' },
    { title: t('模型'), dataIndex: 'model' },
    {
      title: t('当前计费'),
      dataIndex: 'current',
      render: (text) => <div style={{ whiteSpace: 'pre-wrap' }}>{text}</div>,
    },
    {
      title: t('修改为'),
      dataIndex: 'newVal',
      render: (text) => <div style={{ whiteSpace: 'pre-wrap' }}>{text}</div>,
    },
  ];

  return (
    <Modal
      title={t('确认冲突项修改')}
      visible={visible}
      confirmLoading={loading}
      cancelButtonProps={{ disabled: loading }}
      maskClosable={!loading}
      onCancel={loading ? undefined : onCancel}
      onOk={onOk}
      size={isMobile ? 'full-width' : 'large'}
    >
      <Table
        columns={columns}
        dataSource={items}
        pagination={false}
        size='small'
      />
    </Modal>
  );
}

export default function UpstreamRatioSync(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [syncLoading, setSyncLoading] = useState(false);
  const [configLoading, setConfigLoading] = useState(false);
  const [confirmLoading, setConfirmLoading] = useState(false);

  const [allChannels, setAllChannels] = useState([]);
  const [channelsLoaded, setChannelsLoaded] = useState(false);
  const [syncConfig, setSyncConfig] = useState({
    strategy: 'highest',
    sources: [],
    version: 0,
  });

  // 差异数据和测试结果
  const [differences, setDifferences] = useState({});
  const [resolutions, setResolutions] = useState({});

  // 是否已经执行过同步
  const [hasSynced, setHasSynced] = useState(false);

  // 分页相关状态
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // 搜索相关状态
  const [searchKeyword, setSearchKeyword] = useState('');

  // 倍率类型过滤
  const [ratioTypeFilter, setRatioTypeFilter] = useState('');

  // 冲突确认弹窗相关
  const [confirmVisible, setConfirmVisible] = useState(false);
  const [conflictItems, setConflictItems] = useState([]); // {channel, model, current, newVal, ratioType}

  useEffect(() => {
    setCurrentPage(1);
  }, [ratioTypeFilter, searchKeyword]);

  useEffect(() => {
    fetchAllChannels();
    fetchSyncConfig();
  }, []);

  const fetchAllChannels = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/ratio_sync/channels');

      if (res.data.success) {
        const channels = res.data.data || [];

        setAllChannels(channels);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取渠道失败：') + error.message);
    } finally {
      setLoading(false);
      setChannelsLoaded(true);
    }
  };

  const fetchSyncConfig = async () => {
    setConfigLoading(true);
    try {
      const res = await API.get('/api/ratio_sync/config');
      if (res.data.success) {
        setSyncConfig({
          strategy: res.data.data?.strategy || 'highest',
          sources: res.data.data?.sources || [],
          version: res.data.data?.version || 0,
        });
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取同步设置失败：') + error.message);
    } finally {
      setConfigLoading(false);
    }
  };

  const sourceForChannel = (channel) =>
    syncConfig.sources.find((source) => source.channel_id === channel.id);

  const updateSource = (channel, patch) => {
    const current = sourceForChannel(channel) || {
      channel_id: channel.id,
      enabled: false,
      endpoint: defaultEndpointForChannel(channel),
      interval_seconds: 0,
    };
    setSyncConfig((prev) => ({
      ...prev,
      sources: [
        ...prev.sources.filter((source) => source.channel_id !== channel.id),
        { ...current, ...patch },
      ],
    }));
  };

  const saveSyncConfig = async () => {
    setConfigLoading(true);
    try {
      const config = {
        ...syncConfig,
        sources: syncConfig.sources.filter((source) =>
          allChannels.some((channel) => channel.id === source.channel_id),
        ),
      };
      const { version, ...payload } = config;
      const res = await API.put('/api/ratio_sync/config', {
        ...payload,
        expected_version: version,
      });
      if (!res.data.success) {
        showError(res.data.message || t('保存失败'));
        return;
      }
      showSuccess(t('同步设置已保存'));
      fetchSyncConfig();
    } catch (error) {
      if (error?.response?.status === 409) {
        await fetchSyncConfig();
      }
      showError(t('保存失败：') + error.message);
    } finally {
      setConfigLoading(false);
    }
  };

  const fetchRatiosFromChannels = async () => {
    const channelList = allChannels.filter(
      (channel) => sourceForChannel(channel)?.enabled,
    );
    if (channelList.length === 0) {
      showWarning(t('请至少选择一个渠道'));
      return;
    }
    setSyncLoading(true);

    const upstreams = channelList.map((ch) => ({
      id: ch.id,
      name: ch.name,
      base_url: ch.base_url,
      endpoint: sourceForChannel(ch)?.endpoint || defaultEndpointForChannel(ch),
    }));

    const payload = {
      upstreams: upstreams,
      timeout: 10,
    };

    try {
      const res = await API.post('/api/ratio_sync/fetch', payload);

      if (!res.data.success) {
        showError(res.data.message || t('后端请求失败'));
        setSyncLoading(false);
        return;
      }

      const { differences = {}, test_results = [] } = res.data.data;

      const errorResults = test_results.filter((r) => r.status === 'error');
      if (errorResults.length > 0) {
        showWarning(
          t('部分渠道测试失败：') +
            errorResults.map((r) => `${r.name}: ${r.error}`).join(', '),
        );
      }

      const warningResults = test_results.filter(
        (r) => (r.warnings?.length ?? 0) > 0,
      );
      if (warningResults.length > 0) {
        const warningMsg = warningResults
          .map((r) => `${r.name}: ${r.warnings?.join(', ')}`)
          .join('; ');
        showWarning(
          t('Unsupported or invalid pricing skipped: {{warningMsg}}', {
            warningMsg,
          }),
        );
      }

      setDifferences(differences);
      setResolutions({});
      setHasSynced(true);

      if (Object.keys(differences).length === 0) {
        showSuccess(t('未找到差异化价格，无需同步'));
      }
    } catch (e) {
      showError(t('请求后端接口失败：') + e.message);
    } finally {
      setSyncLoading(false);
    }
  };

  const ratioSyncFields = [
    'model_ratio',
    'completion_ratio',
    'cache_ratio',
    'create_cache_ratio',
    'image_ratio',
    'audio_ratio',
    'audio_completion_ratio',
  ];

  const numericSyncFields = new Set([...ratioSyncFields, 'model_price']);
  const syncFieldOrder = [
    ...ratioSyncFields,
    'model_price',
    'billing_mode',
    'billing_expr',
  ];

  function getSyncFieldLabel(ratioType) {
    const typeMap = {
      model_ratio: t('模型倍率'),
      completion_ratio: t('补全倍率'),
      cache_ratio: t('缓存倍率'),
      create_cache_ratio: t('缓存创建倍率'),
      image_ratio: t('图片倍率'),
      audio_ratio: t('音频倍率'),
      audio_completion_ratio: t('音频补全倍率'),
      model_price: t('固定价格'),
      billing_mode: t('计费模式'),
      billing_expr: t('表达式计费'),
    };
    return typeMap[ratioType] || ratioType;
  }

  function getOrderedRatioTypes(ratioTypes) {
    const keys = Object.keys(ratioTypes || {});
    const ordered = [
      ...syncFieldOrder.filter((field) => keys.includes(field)),
      ...keys.filter((field) => !syncFieldOrder.includes(field)),
    ];
    return ratioTypeFilter
      ? ordered.filter((field) => field === ratioTypeFilter)
      : ordered;
  }

  function deleteResolutionField(newRes, model, ratioType) {
    if (!newRes[model]) return;
    delete newRes[model][ratioType];
    if (ratioType === 'billing_expr') {
      delete newRes[model].billing_mode;
    }
    if (ratioType === 'billing_mode') {
      delete newRes[model].billing_expr;
    }
    if (Object.keys(newRes[model]).length === 0) {
      delete newRes[model];
    }
  }

  function getBillingCategory(ratioType) {
    if (ratioType === 'model_price') return 'price';
    if (ratioType === 'billing_mode' || ratioType === 'billing_expr') {
      return 'tiered';
    }
    return 'ratio';
  }

  function optionKeyBySyncField(ratioType) {
    const explicit = {
      billing_mode: 'billing_setting.billing_mode',
      billing_expr: 'billing_setting.billing_expr',
    };
    if (explicit[ratioType]) return explicit[ratioType];
    return ratioType
      .split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join('');
  }

  function getUpstreamValue(model, ratioType, sourceName) {
    return differences[model]?.[ratioType]?.upstreams?.[sourceName];
  }

  function isSelectableUpstreamValue(value) {
    return value !== null && value !== undefined && value !== 'same';
  }

  function getPreferredSyncField(model, ratioType, sourceName) {
    const exprValue = getUpstreamValue(model, 'billing_expr', sourceName);
    if (ratioType !== 'billing_expr' && isSelectableUpstreamValue(exprValue)) {
      return 'billing_expr';
    }
    return ratioType;
  }

  function shouldShowSyncField(model, ratioType, sourceName) {
    if (!sourceName) return true;
    return getPreferredSyncField(model, ratioType, sourceName) === ratioType;
  }

  const selectValue = useCallback(
    (model, ratioType, value, sourceName) => {
      const preferredRatioType = sourceName
        ? getPreferredSyncField(model, ratioType, sourceName)
        : ratioType;
      const preferredValue =
        preferredRatioType === ratioType
          ? value
          : getUpstreamValue(model, preferredRatioType, sourceName);
      ratioType = preferredRatioType;
      value = preferredValue;

      const category = getBillingCategory(ratioType);

      setResolutions((prev) => {
        const newModelRes = { ...(prev[model] || {}) };

        Object.keys(newModelRes).forEach((rt) => {
          if (
            category !== 'tiered' &&
            getBillingCategory(rt) !== 'tiered' &&
            getBillingCategory(rt) !== category
          ) {
            delete newModelRes[rt];
          }
        });

        newModelRes[ratioType] = value;

        if (category === 'tiered' && sourceName) {
          const modeValue =
            differences[model]?.billing_mode?.upstreams?.[sourceName];
          const exprValue =
            differences[model]?.billing_expr?.upstreams?.[sourceName];
          if (
            modeValue !== undefined &&
            modeValue !== null &&
            modeValue !== 'same'
          ) {
            newModelRes.billing_mode = modeValue;
          } else if (ratioType === 'billing_expr') {
            newModelRes.billing_mode = 'tiered_expr';
          }
          if (
            exprValue !== undefined &&
            exprValue !== null &&
            exprValue !== 'same'
          ) {
            newModelRes.billing_expr = exprValue;
          }
        }

        return {
          ...prev,
          [model]: newModelRes,
        };
      });
    },
    [setResolutions, differences],
  );

  const applySync = async () => {
    const currentRatios = {
      ModelRatio: JSON.parse(props.options.ModelRatio || '{}'),
      CompletionRatio: JSON.parse(props.options.CompletionRatio || '{}'),
      CacheRatio: JSON.parse(props.options.CacheRatio || '{}'),
      CreateCacheRatio: JSON.parse(props.options.CreateCacheRatio || '{}'),
      ImageRatio: JSON.parse(props.options.ImageRatio || '{}'),
      AudioRatio: JSON.parse(props.options.AudioRatio || '{}'),
      AudioCompletionRatio: JSON.parse(
        props.options.AudioCompletionRatio || '{}',
      ),
      ModelPrice: JSON.parse(props.options.ModelPrice || '{}'),
      'billing_setting.billing_mode': JSON.parse(
        props.options['billing_setting.billing_mode'] || '{}',
      ),
      'billing_setting.billing_expr': JSON.parse(
        props.options['billing_setting.billing_expr'] || '{}',
      ),
    };

    const conflicts = [];

    const getLocalBillingCategory = (model) => {
      if (currentRatios.ModelPrice[model] !== undefined) return 'price';
      if (
        currentRatios.ModelRatio[model] !== undefined ||
        currentRatios.CompletionRatio[model] !== undefined ||
        currentRatios.CacheRatio[model] !== undefined ||
        currentRatios.CreateCacheRatio[model] !== undefined ||
        currentRatios.ImageRatio[model] !== undefined ||
        currentRatios.AudioRatio[model] !== undefined ||
        currentRatios.AudioCompletionRatio[model] !== undefined
      )
        return 'ratio';
      return null;
    };

    const findSourceChannel = (model, ratioType, value) => {
      if (differences[model] && differences[model][ratioType]) {
        const upMap = differences[model][ratioType].upstreams || {};
        const entry = Object.entries(upMap).find(([_, v]) => v === value);
        if (entry) return entry[0];
      }
      return t('未知');
    };

    Object.entries(resolutions).forEach(([model, ratios]) => {
      const localCat = getLocalBillingCategory(model);
      const newCat =
        'model_price' in ratios
          ? 'price'
          : ratioSyncFields.some((rt) => rt in ratios)
            ? 'ratio'
            : 'tiered';

      if (localCat && newCat !== 'tiered' && localCat !== newCat) {
        const currentDesc =
          localCat === 'price'
            ? `${t('固定价格')} : ${currentRatios.ModelPrice[model]}`
            : `${t('模型倍率')} : ${currentRatios.ModelRatio[model] ?? '-'}\n${t('补全倍率')} : ${currentRatios.CompletionRatio[model] ?? '-'}`;

        let newDesc = '';
        if (newCat === 'price') {
          newDesc = `${t('固定价格')} : ${ratios['model_price']}`;
        } else {
          const newModelRatio = ratios['model_ratio'] ?? '-';
          const newCompRatio = ratios['completion_ratio'] ?? '-';
          newDesc = `${t('模型倍率')} : ${newModelRatio}\n${t('补全倍率')} : ${newCompRatio}`;
        }

        const channels = Object.entries(ratios)
          .map(([rt, val]) => findSourceChannel(model, rt, val))
          .filter((v, idx, arr) => arr.indexOf(v) === idx)
          .join(', ');

        conflicts.push({
          channel: channels,
          model,
          current: currentDesc,
          newVal: newDesc,
        });
      }
    });

    if (conflicts.length > 0) {
      setConflictItems(conflicts);
      setConfirmVisible(true);
      return;
    }

    await performSync(currentRatios);
  };

  const performSync = useCallback(
    async (currentRatios) => {
      const patches = {};
      const ratioOptionKeys = [
        'ModelRatio',
        'CompletionRatio',
        'CacheRatio',
        'CreateCacheRatio',
        'ImageRatio',
        'AudioRatio',
        'AudioCompletionRatio',
      ];
      const tieredOptionKeys = [
        'billing_setting.billing_mode',
        'billing_setting.billing_expr',
      ];
      const deleteModel = (optionKey, model) => {
        const patch = patches[optionKey] || {};
        patch.delete = [...(patch.delete || []), model];
        patches[optionKey] = patch;
      };
      const setModel = (optionKey, model, value) => {
        const patch = patches[optionKey] || {};
        patch.set = { ...(patch.set || {}), [model]: value };
        patches[optionKey] = patch;
      };

      Object.entries(resolutions).forEach(([model, ratios]) => {
        const selectedTypes = Object.keys(ratios);
        const hasPrice = selectedTypes.includes('model_price');
        const hasRatio = selectedTypes.some((rt) =>
          ratioSyncFields.includes(rt),
        );

        if (hasPrice) {
          for (const key of [...ratioOptionKeys, ...tieredOptionKeys]) {
            deleteModel(key, model);
          }
        } else if (hasRatio) {
          deleteModel('ModelPrice', model);
          for (const key of tieredOptionKeys) deleteModel(key, model);
        } else {
          deleteModel('ModelPrice', model);
          for (const key of ratioOptionKeys) deleteModel(key, model);
        }

        Object.entries(ratios).forEach(([ratioType, value]) => {
          const optionKey = optionKeyBySyncField(ratioType);
          setModel(
            optionKey,
            model,
            numericSyncFields.has(ratioType) ? parseFloat(value) : value,
          );
        });
      });

      setLoading(true);
      showInfo(t('正在同步价格，请稍候'));
      let success = false;
      try {
        const result = await API.post('/api/ratio_sync/apply', { patches });

        if (result.data.success) {
          showSuccess(t('同步成功'));
          props.refresh();

          setDifferences((prevDifferences) => {
            const newDifferences = { ...prevDifferences };

            Object.entries(resolutions).forEach(([model, ratios]) => {
              Object.keys(ratios).forEach((ratioType) => {
                if (newDifferences[model] && newDifferences[model][ratioType]) {
                  delete newDifferences[model][ratioType];

                  if (Object.keys(newDifferences[model]).length === 0) {
                    delete newDifferences[model];
                  }
                }
              });
            });

            return newDifferences;
          });

          setResolutions({});
          success = true;
        } else {
          showError(t('部分保存失败'));
        }
      } catch (error) {
        showError(t('保存失败'));
      } finally {
        setLoading(false);
      }
      return success;
    },
    [resolutions, props.options, props.refresh],
  );

  const getCurrentPageData = (dataSource) => {
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    return dataSource.slice(startIndex, endIndex);
  };

  const renderSyncSources = () => {
    const endpointOptions = [
      { label: 'pricing', value: PRICING_ENDPOINT },
      { label: 'ratio_config', value: RATIO_CONFIG_ENDPOINT },
      { label: 'OpenRouter', value: OPENROUTER_ENDPOINT },
      { label: t('自定义'), value: 'custom' },
    ];
    const knownEndpoints = endpointOptions
      .filter((option) => option.value !== 'custom')
      .map((option) => option.value);

    return (
      <Form.Section text={t('选择同步渠道')}>
        <div className='mb-4 flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
          <div className='text-sm text-gray-500'>{t('配置上游价格同步')}</div>
          <div className='flex flex-col gap-2 sm:flex-row'>
            <Select
              value={syncConfig.strategy}
              disabled={configLoading || syncLoading}
              className='w-full sm:w-44'
              onChange={(strategy) =>
                setSyncConfig((prev) => ({ ...prev, strategy }))
              }
            >
              <Select.Option value='highest'>{t('最高价格')}</Select.Option>
              <Select.Option value='lowest'>{t('最低价格')}</Select.Option>
              <Select.Option value='average'>{t('平均价格')}</Select.Option>
            </Select>
            <Button
              type='primary'
              loading={configLoading}
              disabled={syncLoading || !channelsLoaded}
              onClick={saveSyncConfig}
            >
              {t('保存设置')}
            </Button>
            <Button
              icon={<RefreshCcw size={14} />}
              loading={syncLoading}
              disabled={configLoading || !channelsLoaded}
              onClick={fetchRatiosFromChannels}
            >
              {t('检查价格')}
            </Button>
          </div>
        </div>
        <Table
          rowKey='id'
          size='small'
          loading={
            !channelsLoaded || (configLoading && allChannels.length === 0)
          }
          pagination={{ pageSize: 10, showSizeChanger: true }}
          scroll={{ x: 'max-content' }}
          dataSource={allChannels}
          columns={[
            {
              title: t('启用'),
              width: 72,
              render: (_, channel) => {
                const source = sourceForChannel(channel);
                return (
                  <Checkbox
                    checked={source?.enabled || false}
                    disabled={configLoading || syncLoading}
                    onChange={(event) =>
                      updateSource(channel, { enabled: event.target.checked })
                    }
                  />
                );
              },
            },
            { title: t('渠道'), dataIndex: 'name', width: 180 },
            {
              title: t('源地址'),
              dataIndex: 'base_url',
              width: 280,
              render: (url) => (
                <Tooltip content={url}>
                  <span className='inline-block max-w-[260px] truncate font-mono text-xs'>
                    {url}
                  </span>
                </Tooltip>
              ),
            },
            {
              title: t('状态'),
              dataIndex: 'status',
              width: 110,
              render: (status) => (
                <Tag color={status === 1 ? 'green' : 'grey'} shape='circle'>
                  {status === 1 ? t('已启用') : t('未启用')}
                </Tag>
              ),
            },
            {
              title: t('同步接口'),
              width: 260,
              render: (_, channel) => {
                const source = sourceForChannel(channel);
                const endpoint =
                  source?.endpoint || defaultEndpointForChannel(channel);
                const isCustom = !knownEndpoints.includes(endpoint);
                return (
                  <div className='flex gap-2'>
                    <Select
                      value={isCustom ? 'custom' : endpoint}
                      disabled={
                        configLoading || syncLoading || !source?.enabled
                      }
                      className='w-32'
                      onChange={(value) =>
                        updateSource(channel, {
                          endpoint: value === 'custom' ? '' : value,
                        })
                      }
                    >
                      {endpointOptions.map((option) => (
                        <Select.Option key={option.value} value={option.value}>
                          {option.label}
                        </Select.Option>
                      ))}
                    </Select>
                    {isCustom && (
                      <Input
                        value={endpoint}
                        placeholder='/api/pricing'
                        disabled={
                          configLoading || syncLoading || !source?.enabled
                        }
                        onChange={(value) =>
                          updateSource(channel, { endpoint: value })
                        }
                      />
                    )}
                  </div>
                );
              },
            },
            {
              title: t('自动更新'),
              width: 180,
              render: (_, channel) => {
                const source = sourceForChannel(channel);
                return (
                  <Select
                    value={String(source?.interval_seconds || 0)}
                    disabled={configLoading || syncLoading || !source?.enabled}
                    className='w-36'
                    onChange={(value) =>
                      updateSource(channel, { interval_seconds: Number(value) })
                    }
                  >
                    {syncIntervals.map((option) => (
                      <Select.Option
                        key={option.value}
                        value={String(option.value)}
                      >
                        {t(option.label)}
                      </Select.Option>
                    ))}
                  </Select>
                );
              },
            },
          ]}
        />
      </Form.Section>
    );
  };

  const renderHeader = () => (
    <div className='flex flex-col w-full'>
      <div className='flex flex-col md:flex-row justify-between items-center gap-4 w-full'>
        <div className='flex flex-col md:flex-row gap-2 w-full md:w-auto order-2 md:order-1'>
          {(() => {
            const hasSelections = Object.keys(resolutions).length > 0;

            return (
              <Button
                icon={<CheckSquare size={14} />}
                type='secondary'
                onClick={applySync}
                loading={loading || confirmLoading}
                disabled={
                  !hasSelections || loading || syncLoading || confirmLoading
                }
                className='w-full md:w-auto mt-2'
              >
                {t('应用同步')}
              </Button>
            );
          })()}

          <div className='flex flex-col sm:flex-row gap-2 w-full md:w-auto mt-2'>
            <Input
              prefix={<IconSearch size={14} />}
              placeholder={t('搜索模型名称')}
              value={searchKeyword}
              onChange={setSearchKeyword}
              className='w-full sm:w-64'
              disabled={loading || syncLoading || confirmLoading}
              showClear
            />

            <Select
              placeholder={t('按价格字段筛选')}
              value={ratioTypeFilter}
              onChange={setRatioTypeFilter}
              className='w-full sm:w-48'
              disabled={loading || syncLoading || confirmLoading}
              showClear
              onClear={() => setRatioTypeFilter('')}
            >
              <Select.Option value='model_ratio'>{t('模型倍率')}</Select.Option>
              <Select.Option value='completion_ratio'>
                {t('补全倍率')}
              </Select.Option>
              <Select.Option value='cache_ratio'>{t('缓存倍率')}</Select.Option>
              <Select.Option value='create_cache_ratio'>
                {t('缓存创建倍率')}
              </Select.Option>
              <Select.Option value='image_ratio'>{t('图片倍率')}</Select.Option>
              <Select.Option value='audio_ratio'>{t('音频倍率')}</Select.Option>
              <Select.Option value='audio_completion_ratio'>
                {t('音频补全倍率')}
              </Select.Option>
              <Select.Option value='model_price'>{t('固定价格')}</Select.Option>
              <Select.Option value='billing_expr'>
                {t('表达式计费')}
              </Select.Option>
            </Select>
          </div>
        </div>
      </div>
    </div>
  );

  const renderDifferenceTable = () => {
    const dataSource = useMemo(() => {
      return Object.entries(differences).map(([model, ratioTypes]) => {
        const hasPrice = 'model_price' in ratioTypes;
        const hasOtherRatio = ratioSyncFields.some((rt) => rt in ratioTypes);

        return {
          key: model,
          model,
          ratioTypes,
          billingConflict: hasPrice && hasOtherRatio,
        };
      });
    }, [differences]);

    const filteredDataSource = useMemo(() => {
      if (!searchKeyword.trim() && !ratioTypeFilter) {
        return dataSource;
      }

      return dataSource.filter((item) => {
        const matchesKeyword =
          !searchKeyword.trim() ||
          item.model.toLowerCase().includes(searchKeyword.toLowerCase().trim());

        const matchesRatioType =
          !ratioTypeFilter || ratioTypeFilter in item.ratioTypes;

        return matchesKeyword && matchesRatioType;
      });
    }, [dataSource, searchKeyword, ratioTypeFilter]);

    const upstreamNames = useMemo(() => {
      const set = new Set();
      filteredDataSource.forEach((row) => {
        getOrderedRatioTypes(row.ratioTypes).forEach((ratioType) => {
          Object.keys(row.ratioTypes[ratioType]?.upstreams || {}).forEach(
            (name) => set.add(name),
          );
        });
      });
      return Array.from(set);
    }, [filteredDataSource, ratioTypeFilter]);

    const renderValueTag = (value, color = 'default') => {
      if (value === null || value === undefined) {
        return (
          <Tag color='default' shape='circle'>
            {t('未设置')}
          </Tag>
        );
      }

      const text = String(value);
      return (
        <Tooltip content={text}>
          <Tag color={color} shape='circle'>
            <span className='inline-block max-w-[360px] truncate align-bottom'>
              {text}
            </span>
          </Tag>
        </Tooltip>
      );
    };

    const renderCurrentFields = (record) => {
      const fields = getOrderedRatioTypes(record.ratioTypes);
      return (
        <div className='flex min-w-[260px] flex-col gap-2'>
          {fields.map((ratioType) => (
            <div
              key={ratioType}
              className='flex min-w-0 flex-wrap items-center gap-2'
            >
              <Tag color={stringToColor(ratioType)} shape='circle'>
                {getSyncFieldLabel(ratioType)}
              </Tag>
              {renderValueTag(record.ratioTypes[ratioType]?.current, 'blue')}
            </div>
          ))}
        </div>
      );
    };

    const renderUpstreamField = (record, ratioType, upName) => {
      const diff = record.ratioTypes[ratioType] || {};
      const upstreamVal = diff.upstreams?.[upName];
      const isConfident = diff.confidence?.[upName] !== false;
      const isPreferredField =
        getPreferredSyncField(record.model, ratioType, upName) === ratioType;

      if (upstreamVal === null || upstreamVal === undefined) {
        return renderValueTag(undefined);
      }

      if (upstreamVal === 'same') {
        return (
          <Tag color='blue' shape='circle'>
            {t('与本地相同')}
          </Tag>
        );
      }

      const text = String(upstreamVal);
      const isSelected =
        isPreferredField &&
        resolutions[record.model]?.[ratioType] === upstreamVal;
      const valueNode = isPreferredField ? (
        <Checkbox
          checked={isSelected}
          disabled={loading || syncLoading || confirmLoading}
          onChange={(e) => {
            const isChecked = e.target.checked;
            if (isChecked) {
              selectValue(record.model, ratioType, upstreamVal, upName);
            } else {
              setResolutions((prev) => {
                const newRes = { ...prev };
                deleteResolutionField(newRes, record.model, ratioType);
                return newRes;
              });
            }
          }}
        >
          <Tooltip content={text}>
            <span className='inline-block max-w-[360px] truncate align-bottom'>
              {text}
            </span>
          </Tooltip>
        </Checkbox>
      ) : (
        <Tooltip content={text}>
          <Tag color='default' shape='circle' type='light'>
            <span className='inline-block max-w-[360px] truncate align-bottom'>
              {text}
            </span>
          </Tag>
        </Tooltip>
      );

      return (
        <div className='flex min-w-0 items-center gap-2'>
          {valueNode}
          {!isConfident && (
            <Tooltip
              position='left'
              content={t('该数据可能不可信，请谨慎使用')}
            >
              <AlertTriangle size={16} className='shrink-0 text-yellow-500' />
            </Tooltip>
          )}
        </div>
      );
    };

    const renderUpstreamFields = (record, upName) => {
      const fields = getOrderedRatioTypes(record.ratioTypes).filter(
        (ratioType) => shouldShowSyncField(record.model, ratioType, upName),
      );
      return (
        <div className='flex min-w-[280px] flex-col gap-2'>
          {fields.map((ratioType) => (
            <div key={ratioType} className='flex min-w-0 items-start gap-2'>
              <Tag
                color={stringToColor(ratioType)}
                shape='circle'
                className='shrink-0'
              >
                {getSyncFieldLabel(ratioType)}
              </Tag>
              <div className='min-w-0 flex-1'>
                {renderUpstreamField(record, ratioType, upName)}
              </div>
            </div>
          ))}
        </div>
      );
    };

    if (filteredDataSource.length === 0) {
      if (syncLoading) {
        return (
          <div className='flex min-h-[260px] flex-col items-center justify-center gap-3'>
            <Spin size='large' />
            <div className='text-sm text-gray-500'>
              {t('正在同步上游价格，请稍候')}
            </div>
          </div>
        );
      }

      return (
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={
            searchKeyword.trim()
              ? t('未找到匹配的模型')
              : Object.keys(differences).length === 0
                ? hasSynced
                  ? t('暂无差异化价格显示')
                  : t('请先选择同步渠道')
                : t('请先选择同步渠道')
          }
          style={{ padding: 30 }}
        />
      );
    }

    const columns = [
      {
        title: t('模型'),
        dataIndex: 'model',
        fixed: 'left',
        render: (text, record) => (
          <div className='flex min-w-[180px] items-center gap-2'>
            <span className='font-medium'>{text}</span>
            {record.billingConflict && (
              <Tooltip
                position='top'
                content={t('该模型存在固定价格与倍率计费方式冲突，请确认选择')}
              >
                <AlertTriangle size={14} className='shrink-0 text-yellow-500' />
              </Tooltip>
            )}
          </div>
        ),
      },
      {
        title: t('当前价格'),
        dataIndex: 'current',
        render: (_, record) => renderCurrentFields(record),
      },
      ...upstreamNames.map((upName) => {
        const channelStats = (() => {
          let selectableCount = 0;
          let selectedCount = 0;

          filteredDataSource.forEach((row) => {
            getOrderedRatioTypes(row.ratioTypes).forEach((ratioType) => {
              const upstreamVal =
                row.ratioTypes[ratioType]?.upstreams?.[upName];
              if (
                getPreferredSyncField(row.model, ratioType, upName) ===
                  ratioType &&
                isSelectableUpstreamValue(upstreamVal)
              ) {
                selectableCount++;
                if (resolutions[row.model]?.[ratioType] === upstreamVal) {
                  selectedCount++;
                }
              }
            });
          });

          return {
            selectableCount,
            selectedCount,
            allSelected:
              selectableCount > 0 && selectedCount === selectableCount,
            partiallySelected:
              selectedCount > 0 && selectedCount < selectableCount,
            hasSelectableItems: selectableCount > 0,
          };
        })();

        const handleBulkSelect = (checked) => {
          if (checked) {
            filteredDataSource.forEach((row) => {
              getOrderedRatioTypes(row.ratioTypes).forEach((ratioType) => {
                const upstreamVal =
                  row.ratioTypes[ratioType]?.upstreams?.[upName];
                if (
                  getPreferredSyncField(row.model, ratioType, upName) ===
                    ratioType &&
                  isSelectableUpstreamValue(upstreamVal)
                ) {
                  selectValue(row.model, ratioType, upstreamVal, upName);
                }
              });
            });
          } else {
            setResolutions((prev) => {
              const newRes = { ...prev };
              filteredDataSource.forEach((row) => {
                getOrderedRatioTypes(row.ratioTypes).forEach((ratioType) => {
                  if (
                    row.ratioTypes[ratioType]?.upstreams?.[upName] !== undefined
                  ) {
                    deleteResolutionField(newRes, row.model, ratioType);
                  }
                });
              });
              return newRes;
            });
          }
        };

        return {
          title: channelStats.hasSelectableItems ? (
            <Checkbox
              checked={channelStats.allSelected}
              indeterminate={channelStats.partiallySelected}
              disabled={loading || syncLoading || confirmLoading}
              onChange={(e) => handleBulkSelect(e.target.checked)}
            >
              {upName}
            </Checkbox>
          ) : (
            <span>{upName}</span>
          ),
          dataIndex: upName,
          render: (_, record) => renderUpstreamFields(record, upName),
        };
      }),
    ];

    return (
      <Table
        columns={columns}
        dataSource={getCurrentPageData(filteredDataSource)}
        pagination={{
          currentPage: currentPage,
          pageSize: pageSize,
          total: filteredDataSource.length,
          showSizeChanger: true,
          showQuickJumper: true,
          pageSizeOptions: ['5', '10', '20', '50'],
          onChange: (page, size) => {
            setCurrentPage(page);
            setPageSize(size);
          },
          onShowSizeChange: (current, size) => {
            setCurrentPage(1);
            setPageSize(size);
          },
        }}
        scroll={{ x: 'max-content' }}
        size='middle'
        loading={loading || syncLoading}
      />
    );
  };

  return (
    <>
      {renderSyncSources()}
      <Form.Section text={renderHeader()}>
        {renderDifferenceTable()}
      </Form.Section>

      <ConflictConfirmModal
        t={t}
        visible={confirmVisible}
        items={conflictItems}
        loading={confirmLoading}
        onOk={async () => {
          setConfirmLoading(true);
          const curRatios = {
            ModelRatio: JSON.parse(props.options.ModelRatio || '{}'),
            CompletionRatio: JSON.parse(props.options.CompletionRatio || '{}'),
            CacheRatio: JSON.parse(props.options.CacheRatio || '{}'),
            CreateCacheRatio: JSON.parse(
              props.options.CreateCacheRatio || '{}',
            ),
            ImageRatio: JSON.parse(props.options.ImageRatio || '{}'),
            AudioRatio: JSON.parse(props.options.AudioRatio || '{}'),
            AudioCompletionRatio: JSON.parse(
              props.options.AudioCompletionRatio || '{}',
            ),
            ModelPrice: JSON.parse(props.options.ModelPrice || '{}'),
            'billing_setting.billing_mode': JSON.parse(
              props.options['billing_setting.billing_mode'] || '{}',
            ),
            'billing_setting.billing_expr': JSON.parse(
              props.options['billing_setting.billing_expr'] || '{}',
            ),
          };
          try {
            const success = await performSync(curRatios);
            if (success) {
              setConfirmVisible(false);
            }
          } finally {
            setConfirmLoading(false);
          }
        }}
        onCancel={() => setConfirmVisible(false)}
      />
    </>
  );
}
