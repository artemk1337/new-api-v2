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

import React, { useEffect, useMemo, useState, useRef } from 'react';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
  renderGroupOption,
  getCurrencyConfig,
  getModelCategories,
  selectFilter,
} from '../../../../helpers';
import {
  quotaToDisplayAmount,
  displayAmountToQuota,
} from '../../../../helpers/quota';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Avatar,
  Form,
  Col,
  Row,
  InputNumber,
  Radio,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconLink,
  IconSave,
  IconClose,
  IconKey,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const EditTokenModal = (props) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [autoCandidateGroups, setAutoCandidateGroups] = useState([]);
  const [showQuotaInput, setShowQuotaInput] = useState(false);
  const isEdit = props.editingToken.id !== undefined;

  const getInitValues = () => ({
    name: '',
    remain_quota: 0,
    remain_amount: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: [],
    allow_ips: '',
    group: 'auto',
    auto_group_mode: 'all',
    auto_group_candidates: [],
    tokenCount: 1,
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (!formApiRef.current) return;
    if (seconds !== 0) {
      timestamp += seconds;
      formApiRef.current.setValue('expired_time', timestamp2string(timestamp));
    } else {
      formApiRef.current.setValue('expired_time', -1);
    }
  };

  const loadModels = async () => {
    let res = await API.get(`/api/user/models`);
    const { success, message, data } = res.data;
    if (success) {
      const categories = getModelCategories(t);
      let localModelOptions = data.map((model) => {
        let icon = null;
        for (const [key, category] of Object.entries(categories)) {
          if (key !== 'all' && category.filter({ model_name: model })) {
            icon = category.icon;
            break;
          }
        }
        return {
          label: (
            <span className='flex items-center gap-1'>
              {icon}
              {model}
            </span>
          ),
          value: model,
        };
      });
      setModels(localModelOptions);
    } else {
      showError(t(message));
    }
  };

  const loadGroups = async () => {
    try {
      const res = await API.get(`/api/user/self/groups`);
      const { success, message, data, auto_groups } = res.data;
      if (!success) {
        showError(t(message));
        return;
      }

      const concreteGroups = Object.entries(data)
        .filter(([group]) => group !== 'auto')
        .map(([group, info]) => ({
          label: info.name || info.desc || group,
          fullLabel: info.desc,
          value: group,
          ratio: info.ratio,
        }));
      const concreteByValue = new Map(
        concreteGroups.map((group) => [group.value, group]),
      );
      const seen = new Set();
      const autoGroups = (auto_groups || [])
        .map((group) => String(group))
        .filter((group) => {
          if (
            group === 'auto' ||
            seen.has(group) ||
            !concreteByValue.has(group)
          ) {
            return false;
          }
          seen.add(group);
          return true;
        })
        .map((group) => concreteByValue.get(group));

      setAutoCandidateGroups(autoGroups);
      if (autoGroups.length === 0) {
        setGroups(concreteGroups);
        return;
      }
      setGroups([
        {
          label: 'Auto',
          fullLabelKey: '先尝试最便宜的分组，确认错误未产生费用后再切换。',
          value: 'auto',
          dynamicRatio: true,
        },
        ...concreteGroups,
      ]);
    } catch (error) {
      showError(error.message);
    }
  };

  const loadToken = async (tokenId, isCurrent) => {
    setLoading(true);
    try {
      let res = await API.get(`/api/token/${tokenId}`);
      if (!isCurrent()) {
        return;
      }
      const { success, message, data } = res.data;
      if (success) {
        if (data.expired_time !== -1) {
          data.expired_time = timestamp2string(data.expired_time);
        }
        if (data.model_limits !== '') {
          data.model_limits = data.model_limits.split(',');
        } else {
          data.model_limits = [];
        }
        data.group = data.group || 'auto';
        if (typeof data.auto_group_candidates === 'string') {
          data.auto_group_candidates = data.auto_group_candidates
            .split(',')
            .map((group) => group.trim())
            .filter(Boolean);
        } else if (!Array.isArray(data.auto_group_candidates)) {
          data.auto_group_candidates = [];
        }
        data.auto_group_candidates = data.auto_group_candidates.filter(
          (group) => group !== 'auto',
        );
        data.auto_group_mode =
          data.auto_group_candidates.length > 0 ? 'specific' : 'all';
        data.remain_amount = Number(
          quotaToDisplayAmount(data.remain_quota || 0).toFixed(6),
        );
        if (formApiRef.current) {
          formApiRef.current.setValues({ ...getInitValues(), ...data });
        }
      } else {
        showError(message);
      }
    } catch (error) {
      if (isCurrent()) {
        showError(error.message);
      }
    } finally {
      if (isCurrent()) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (formApiRef.current) {
      if (!isEdit) {
        formApiRef.current.setValues(getInitValues());
      }
    }
    loadModels();
    loadGroups();
  }, [props.editingToken.id]);

  useEffect(() => {
    let ignore = false;
    if (props.visiable) {
      if (isEdit) {
        loadToken(props.editingToken.id, () => !ignore);
      } else {
        setLoading(false);
        formApiRef.current?.setValues(getInitValues());
      }
    } else {
      setLoading(false);
      formApiRef.current?.reset();
    }
    return () => {
      ignore = true;
    };
  }, [props.visiable, props.editingToken.id]);

  const generateRandomSuffix = () => {
    const characters =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 6; i++) {
      result += characters.charAt(
        Math.floor(Math.random() * characters.length),
      );
    }
    return result;
  };

  const availableGroupValues = useMemo(
    () => new Set(groups.map((group) => group.value)),
    [groups],
  );
  const autoCandidateValues = useMemo(
    () => new Set(autoCandidateGroups.map((group) => group.value)),
    [autoCandidateGroups],
  );
  const getGroupOptions = (selectedGroup) => {
    if (!selectedGroup || availableGroupValues.has(selectedGroup)) {
      return groups;
    }
    const unavailableGroup = {
      label: `${selectedGroup} (${t('不可用')})`,
      fullLabel: t('此分组已不可用，请选择其他分组。'),
      value: selectedGroup,
    };
    return selectedGroup === 'auto'
      ? [unavailableGroup, ...groups]
      : [...groups, unavailableGroup];
  };
  const getUnavailableAutoCandidates = (values) => {
    const selected = Array.isArray(values.auto_group_candidates)
      ? values.auto_group_candidates
      : [];
    return selected.filter(
      (group) => group === 'auto' || !autoCandidateValues.has(group),
    );
  };
  const getAutoCandidateOptions = (values) => [
    ...autoCandidateGroups,
    ...getUnavailableAutoCandidates(values).map((group) => ({
      label: `${group} (${t('不可用')})`,
      fullLabel: t('保存前请移除此分组。'),
      value: group,
    })),
  ];

  const submit = async (values) => {
    if (!values.group || !availableGroupValues.has(values.group)) {
      showError(t('请选择可用分组'));
      return;
    }
    const autoGroupCandidates = Array.isArray(values.auto_group_candidates)
      ? values.auto_group_candidates.filter((group) => group !== 'auto')
      : [];
    if (
      values.group === 'auto' &&
      values.auto_group_mode === 'specific' &&
      autoGroupCandidates.length === 0
    ) {
      showError(t('请为 Auto 至少选择一个分组'));
      return;
    }
    if (
      values.group === 'auto' &&
      autoGroupCandidates.some((group) => !autoCandidateValues.has(group))
    ) {
      showError(t('保存前请移除不可用分组'));
      return;
    }

    setLoading(true);
    if (isEdit) {
      let {
        tokenCount: _tc,
        auto_group_mode: _autoGroupMode,
        ...localInputs
      } = values;
      localInputs.group = localInputs.group || 'auto';
      localInputs.auto_group_candidates =
        localInputs.group === 'auto' && values.auto_group_mode === 'specific'
          ? autoGroupCandidates
          : [];
      localInputs.remain_quota = localInputs.unlimited_quota
        ? 0
        : displayAmountToQuota(localInputs.remain_amount);
      if (!localInputs.unlimited_quota && localInputs.remain_quota <= 0) {
        showError(t('请输入金额'));
        setLoading(false);
        return;
      }
      if (localInputs.expired_time !== -1) {
        let time = Date.parse(localInputs.expired_time);
        if (isNaN(time)) {
          showError(t('过期时间格式错误！'));
          setLoading(false);
          return;
        }
        localInputs.expired_time = Math.ceil(time / 1000);
      }
      localInputs.model_limits = localInputs.model_limits.join(',');
      localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
      let res = await API.put(`/api/token/`, {
        ...localInputs,
        id: parseInt(props.editingToken.id),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('令牌更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(t(message));
      }
    } else {
      const count = parseInt(values.tokenCount, 10) || 1;
      let successCount = 0;
      for (let i = 0; i < count; i++) {
        let {
          tokenCount: _tc,
          auto_group_mode: _autoGroupMode,
          ...localInputs
        } = values;
        localInputs.group = localInputs.group || 'auto';
        localInputs.auto_group_candidates =
          localInputs.group === 'auto' && values.auto_group_mode === 'specific'
            ? autoGroupCandidates
            : [];
        const baseName =
          values.name.trim() === '' ? 'default' : values.name.trim();
        if (i !== 0 || values.name.trim() === '') {
          localInputs.name = `${baseName}-${generateRandomSuffix()}`;
        } else {
          localInputs.name = baseName;
        }
        localInputs.remain_quota = localInputs.unlimited_quota
          ? 0
          : displayAmountToQuota(localInputs.remain_amount);
        if (!localInputs.unlimited_quota && localInputs.remain_quota <= 0) {
          showError(t('请输入金额'));
          setLoading(false);
          break;
        }

        if (localInputs.expired_time !== -1) {
          let time = Date.parse(localInputs.expired_time);
          if (isNaN(time)) {
            showError(t('过期时间格式错误！'));
            setLoading(false);
            break;
          }
          localInputs.expired_time = Math.ceil(time / 1000);
        }
        localInputs.model_limits = localInputs.model_limits.join(',');
        localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
        let res = await API.post(`/api/token/`, localInputs);
        const { success, message } = res.data;
        if (success) {
          successCount++;
        } else {
          showError(t(message));
          break;
        }
      }
      if (successCount > 0) {
        showSuccess(t('令牌创建成功，请在列表页面点击复制获取令牌！'));
        props.refresh();
        props.handleClose();
      }
    }
    setLoading(false);
    formApiRef.current?.setValues(getInitValues());
  };

  return (
    <SideSheet
      placement={isEdit ? 'right' : 'left'}
      title={
        <Space>
          {isEdit ? (
            <Tag color='blue' shape='circle'>
              {t('更新')}
            </Tag>
          ) : (
            <Tag color='green' shape='circle'>
              {t('新建')}
            </Tag>
          )}
          <Title heading={4} className='m-0'>
            {isEdit ? t('更新令牌信息') : t('创建新的令牌')}
          </Title>
        </Space>
      }
      bodyStyle={{ padding: '0' }}
      visible={props.visiable}
      width={isMobile ? '100%' : 600}
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button
              theme='solid'
              className='!rounded-lg'
              onClick={() => formApiRef.current?.submitForm()}
              icon={<IconSave />}
              loading={loading}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              className='!rounded-lg'
              type='primary'
              onClick={handleCancel}
              icon={<IconClose />}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      }
      closeIcon={null}
      onCancel={() => handleCancel()}
    >
      <Spin spinning={loading}>
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {({ values }) => (
            <div className='p-2'>
              {/* 基本信息 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconKey size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的基本信息')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Input
                      field='name'
                      label={t('名称')}
                      placeholder={t('请输入名称')}
                      rules={[{ required: true, message: t('请输入名称') }]}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    {groups.length > 0 ? (
                      <>
                        <Form.Select
                          field='group'
                          label={t('令牌分组')}
                          placeholder={t('请选择分组')}
                          optionList={getGroupOptions(values.group)}
                          renderOptionItem={renderGroupOption}
                          rules={[{ required: true, message: t('请选择分组') }]}
                          extraText={t('分组影响渠道稳定性和模型可用性。')}
                          onChange={(value) => {
                            if (value !== 'auto') {
                              formApiRef.current?.setValue(
                                'auto_group_candidates',
                                [],
                              );
                              formApiRef.current?.setValue(
                                'auto_group_mode',
                                'all',
                              );
                            }
                          }}
                          filter={(input, option) => {
                            const q = input.toLowerCase();
                            return (
                              option.value?.toLowerCase().includes(q) ||
                              (typeof option.label === 'string' &&
                                option.label.toLowerCase().includes(q))
                            );
                          }}
                          style={{ width: '100%' }}
                        />
                        {values.group &&
                          !availableGroupValues.has(values.group) && (
                            <Text type='danger' size='small'>
                              {t('此分组已不可用，请选择其他分组。')}
                            </Text>
                          )}
                      </>
                    ) : (
                      <Form.Select
                        placeholder={t('管理员未设置用户可选分组')}
                        disabled
                        label={t('令牌分组')}
                        style={{ width: '100%' }}
                      />
                    )}
                  </Col>
                  <Col
                    span={24}
                    style={{
                      display:
                        values.group === 'auto' &&
                        availableGroupValues.has('auto')
                          ? 'block'
                          : 'none',
                    }}
                  >
                    <Card
                      className='!rounded-xl'
                      style={{
                        background: 'var(--semi-color-fill-0)',
                        marginBottom: 12,
                      }}
                    >
                      <Form.RadioGroup
                        field='auto_group_mode'
                        label={t('Auto 可用分组')}
                        type='button'
                        direction='horizontal'
                        onChange={(value) => {
                          if (value === 'all') {
                            formApiRef.current?.setValue(
                              'auto_group_candidates',
                              [],
                            );
                          }
                        }}
                      >
                        <Radio value='all'>{t('全部分组')}</Radio>
                        <Radio value='specific'>{t('指定分组')}</Radio>
                      </Form.RadioGroup>

                      <div
                        style={{
                          display:
                            values.auto_group_mode === 'specific'
                              ? 'block'
                              : 'none',
                        }}
                      >
                        <Form.Select
                          field='auto_group_candidates'
                          label={t('Auto 分组候选')}
                          placeholder={t('请选择 Auto 可使用的分组')}
                          multiple
                          optionList={getAutoCandidateOptions(values)}
                          renderOptionItem={renderGroupOption}
                          onChange={(selected) => {
                            const next = Array.isArray(selected)
                              ? selected.filter((group) => group !== 'auto')
                              : [];
                            formApiRef.current?.setValue(
                              'auto_group_candidates',
                              next,
                            );
                          }}
                          rules={
                            values.auto_group_mode === 'specific'
                              ? [
                                  {
                                    required: true,
                                    message: t('请为 Auto 至少选择一个分组'),
                                  },
                                ]
                              : []
                          }
                          style={{ width: '100%' }}
                        />
                      </div>

                      {getUnavailableAutoCandidates(values).length > 0 && (
                        <Text type='danger' size='small'>
                          {t(
                            '以下已保存分组不再可用：{{groups}}。保存前请将其移除。',
                            {
                              groups:
                                getUnavailableAutoCandidates(values).join(', '),
                            },
                          )}
                        </Text>
                      )}

                      <div
                        className='mt-2'
                        style={{
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 4,
                        }}
                      >
                        <Text type='tertiary' size='small'>
                          {t(
                            'Auto 优先使用所选分组中最便宜的分组，仅在确认未产生费用的错误后切换。',
                          )}
                        </Text>
                        <Text type='tertiary' size='small'>
                          {t(
                            '请求前会按所选最贵分组预留额度，最终只扣除实际费用。',
                          )}
                        </Text>
                        <Text type='tertiary' size='small'>
                          {t('若预留额度过高，请充值或仅保留更便宜的分组。')}
                        </Text>
                      </div>
                    </Card>
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                    <Form.DatePicker
                      field='expired_time'
                      label={t('过期时间')}
                      type='dateTime'
                      placeholder={t('请选择过期时间')}
                      rules={[
                        { required: true, message: t('请选择过期时间') },
                        {
                          validator: (rule, value) => {
                            // 允许 -1 表示永不过期，也允许空值在必填校验时被拦截
                            if (value === -1 || !value)
                              return Promise.resolve();
                            const time = Date.parse(value);
                            if (isNaN(time)) {
                              return Promise.reject(t('过期时间格式错误！'));
                            }
                            if (time <= Date.now()) {
                              return Promise.reject(
                                t('过期时间不能早于当前时间！'),
                              );
                            }
                            return Promise.resolve();
                          },
                        },
                      ]}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                    <Form.Slot label={t('过期时间快捷设置')}>
                      <Space wrap>
                        <Button
                          theme='light'
                          type='primary'
                          onClick={() => setExpiredTime(0, 0, 0, 0)}
                        >
                          {t('永不过期')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(1, 0, 0, 0)}
                        >
                          {t('一个月')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 1, 0, 0)}
                        >
                          {t('一天')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 0, 1, 0)}
                        >
                          {t('一小时')}
                        </Button>
                      </Space>
                    </Form.Slot>
                  </Col>
                  {!isEdit && (
                    <Col span={24}>
                      <Form.InputNumber
                        field='tokenCount'
                        label={t('新建数量')}
                        min={1}
                        extraText={t('批量创建时会在名称后自动添加随机后缀')}
                        rules={[
                          { required: true, message: t('请输入新建数量') },
                        ]}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  )}
                </Row>
              </Card>

              {/* 额度设置 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <IconCreditCard size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('额度设置')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌可用额度和数量')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.InputNumber
                      field='remain_amount'
                      label={t('金额')}
                      prefix={getCurrencyConfig().symbol}
                      placeholder={t('输入金额')}
                      precision={6}
                      disabled={values.unlimited_quota}
                      min={0}
                      step={0.000001}
                      onChange={(val) => {
                        const amount = val === '' || val == null ? 0 : val;
                        formApiRef.current?.setValue('remain_amount', amount);
                        formApiRef.current?.setValue(
                          'remain_quota',
                          displayAmountToQuota(amount),
                        );
                      }}
                      style={{ width: '100%' }}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    <div
                      className='text-xs cursor-pointer mt-1'
                      style={{ color: 'var(--semi-color-text-2)' }}
                      onClick={() => setShowQuotaInput((v) => !v)}
                    >
                      {showQuotaInput
                        ? `▾ ${t('收起原生额度输入')}`
                        : `▸ ${t('使用原生额度输入')}`}
                    </div>
                    <div
                      style={{ display: showQuotaInput ? 'block' : 'none' }}
                      className='mt-2'
                    >
                      <Form.InputNumber
                        field='remain_quota'
                        label={t('额度')}
                        placeholder={t('输入额度')}
                        disabled={values.unlimited_quota}
                        min={0}
                        step={500000}
                        rules={
                          values.unlimited_quota
                            ? []
                            : [{ required: true, message: t('请输入额度') }]
                        }
                        onChange={(val) => {
                          const quota = val === '' || val == null ? 0 : val;
                          formApiRef.current?.setValue('remain_quota', quota);
                          formApiRef.current?.setValue(
                            'remain_amount',
                            Number(quotaToDisplayAmount(quota).toFixed(6)),
                          );
                        }}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </div>
                  </Col>
                  <Col span={24}>
                    <Form.Switch
                      field='unlimited_quota'
                      label={t('无限额度')}
                      size='default'
                      extraText={t(
                        '令牌的额度仅用于限制令牌本身的最大额度使用量，实际的使用受到账户的剩余额度限制',
                      )}
                    />
                  </Col>
                </Row>
              </Card>

              {/* 访问限制 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar
                    size='small'
                    color='purple'
                    className='mr-2 shadow-md'
                  >
                    <IconLink size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('访问限制')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的访问限制')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Select
                      field='model_limits'
                      label={t('模型限制列表')}
                      placeholder={t(
                        '请选择该令牌支持的模型，留空支持所有模型',
                      )}
                      multiple
                      optionList={models}
                      extraText={t('非必要，不建议启用模型限制')}
                      filter={selectFilter}
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.TextArea
                      field='allow_ips'
                      label={t('IP白名单（支持CIDR表达式）')}
                      placeholder={t('允许的IP，一行一个，不填写则不限制')}
                      autosize
                      rows={1}
                      extraText={t(
                        '请勿过度信任此功能，IP可能被伪造，请配合nginx和cdn等网关使用',
                      )}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>
            </div>
          )}
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditTokenModal;
