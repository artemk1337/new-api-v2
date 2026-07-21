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
import { API, showError, showSuccess } from '../../helpers';
import {
  Button,
  Card,
  Divider,
  Form,
  Input,
  Typography,
} from '@douyinfe/semi-ui';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';

const { Title, Text, Paragraph } = Typography;

const TwoFAVerification = ({ onSuccess, onBack, isModal = false }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [useBackupCode, setUseBackupCode] = useState(false);
  const [verificationCode, setVerificationCode] = useState('');

  const handleSubmit = async () => {
    if (!verificationCode) {
      showError(t('请输入验证码'));
      return;
    }
    // Validate code format
    if (useBackupCode && verificationCode.length !== 8) {
      showError('Резервный код должен содержать 8 символов');
      return;
    } else if (!useBackupCode && !/^\d{6}$/.test(verificationCode)) {
      showError('Код подтверждения должен состоять из 6 цифр');
      return;
    }

    setLoading(true);
    try {
      const res = await API.post('/api/user/login/2fa', {
        code: verificationCode,
      });

      if (res.data.success) {
        showSuccess(t('登录成功！'));
        // 保存用户信息到本地存储
        localStorage.setItem('user', JSON.stringify(res.data.data));
        if (onSuccess) {
          onSuccess(res.data.data);
        }
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('验证失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const handleKeyPress = (e) => {
    if (e.key === 'Enter') {
      handleSubmit();
    }
  };

  if (isModal) {
    return (
      <div className='space-y-4'>
        <Paragraph className='text-gray-600 dark:text-gray-300'>
          {t('输入认证器应用显示的6位数字验证码')}
        </Paragraph>

        <Form onSubmit={handleSubmit}>
          <Form.Input
            field='code'
            label={useBackupCode ? 'Резервный код' : t('验证码')}
            placeholder={useBackupCode ? 'Введите 8-значный резервный код' : 'Введите 6-значный код подтверждения'}
            value={verificationCode}
            onChange={setVerificationCode}
            onKeyPress={handleKeyPress}
            size='large'
            style={{ marginBottom: 16 }}
            autoFocus
          />

          <Button
            htmlType='submit'
            type='primary'
            loading={loading}
            block
            size='large'
            style={{ marginBottom: 16 }}
          >
            Подтвердить и войти
          </Button>
        </Form>

        <Divider />

        <div style={{ textAlign: 'center' }}>
          <Button
            theme='borderless'
            type='tertiary'
            onClick={() => {
              setUseBackupCode(!useBackupCode);
              setVerificationCode('');
            }}
            style={{ marginRight: 16, color: '#1890ff', padding: 0 }}
          >
            {useBackupCode ? 'Использовать код аутентификатора' : 'Использовать резервный код'}
          </Button>

          {onBack && (
            <Button
              theme='borderless'
              type='tertiary'
              onClick={onBack}
              style={{ color: '#1890ff', padding: 0 }}
            >
              Вернуться ко входу
            </Button>
          )}
        </div>

        <div className='bg-gray-50 dark:bg-gray-800 rounded-lg p-3'>
          <Text size='small' type='secondary'>
            <strong>Подсказка:</strong>
            <br />
            • Код подтверждения обновляется каждые 30 секунд
            <br />
            • Если не удается получить код, используйте резервный код
            <br />• Каждый резервный код можно использовать только один раз
          </Text>
        </div>
      </div>
    );
  }

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '60vh',
      }}
    >
      <Card style={{ width: 400, padding: 24 }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Title heading={3}>{t('两步验证')}</Title>
          <Paragraph type='secondary'>
            {t('输入认证器应用显示的6位数字验证码')}
          </Paragraph>
        </div>

        <Form onSubmit={handleSubmit}>
          <Form.Input
            field='code'
            label={useBackupCode ? 'Резервный код' : t('验证码')}
            placeholder={useBackupCode ? 'Введите 8-значный резервный код' : 'Введите 6-значный код подтверждения'}
            value={verificationCode}
            onChange={setVerificationCode}
            onKeyPress={handleKeyPress}
            size='large'
            style={{ marginBottom: 16 }}
            autoFocus
          />

          <Button
            htmlType='submit'
            type='primary'
            loading={loading}
            block
            size='large'
            style={{ marginBottom: 16 }}
          >
            Подтвердить и войти
          </Button>
        </Form>

        <Divider />

        <div style={{ textAlign: 'center' }}>
          <Button
            theme='borderless'
            type='tertiary'
            onClick={() => {
              setUseBackupCode(!useBackupCode);
              setVerificationCode('');
            }}
            style={{ marginRight: 16, color: '#1890ff', padding: 0 }}
          >
            {useBackupCode ? 'Использовать код аутентификатора' : 'Использовать резервный код'}
          </Button>

          {onBack && (
            <Button
              theme='borderless'
              type='tertiary'
              onClick={onBack}
              style={{ color: '#1890ff', padding: 0 }}
            >
              Вернуться ко входу
            </Button>
          )}
        </div>

        <div
          style={{
            marginTop: 24,
            padding: 16,
            background: '#f6f8fa',
            borderRadius: 6,
          }}
        >
          <Text size='small' type='secondary'>
            <strong>Подсказка:</strong>
            <br />
            • Код подтверждения обновляется каждые 30 секунд
            <br />
            • Если не удается получить код, используйте резервный код
            <br />• Каждый резервный код можно использовать только один раз
          </Text>
        </div>
      </Card>
    </div>
  );
};

export default TwoFAVerification;
