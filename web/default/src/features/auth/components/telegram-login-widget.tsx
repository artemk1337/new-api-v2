/*
Copyright (C) 2023-2026 QuantumNous

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
*/
import { useEffect, useId, useRef } from 'react'

import { cn } from '@/lib/utils'

export type TelegramLoginData = {
  id: number
  first_name: string
  last_name?: string
  username?: string
  photo_url?: string
  auth_date: number
  hash: string
}

type TelegramLoginWidgetProps = {
  botName: string
  onAuth?: (data: TelegramLoginData) => void | Promise<void>
  authUrl?: string
  className?: string
  onError?: () => void
}

const telegramWidgetURL = 'https://telegram.org/js/telegram-widget.js?22'

export function TelegramLoginWidget({
  botName,
  onAuth,
  authUrl,
  className,
  onError,
}: TelegramLoginWidgetProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const onAuthRef = useRef(onAuth)
  const onErrorRef = useRef(onError)
  const callbackName = `telegramLogin${useId().replaceAll(/[^a-zA-Z0-9]/g, '')}`
  const normalizedBotName = botName.trim().replace(/^@/, '')

  useEffect(() => {
    onAuthRef.current = onAuth
  }, [onAuth])

  useEffect(() => {
    onErrorRef.current = onError
  }, [onError])

  useEffect(() => {
    const container = containerRef.current
    if (!container || !normalizedBotName) return

    const callbacks = window as unknown as Window &
      Record<string, (data: TelegramLoginData) => void>
    const script = document.createElement('script')
    script.src = telegramWidgetURL
    script.async = true
    script.setAttribute('data-telegram-login', normalizedBotName)
    script.setAttribute('data-size', 'large')
    script.setAttribute('data-radius', '8')
    script.setAttribute('data-userpic', 'false')

    if (authUrl) {
      script.setAttribute('data-auth-url', authUrl)
    } else {
      callbacks[callbackName] = (data) => void onAuthRef.current?.(data)
      script.setAttribute('data-onauth', `${callbackName}(user)`)
    }

    script.addEventListener('error', () => onErrorRef.current?.(), {
      once: true,
    })
    container.replaceChildren(script)

    return () => {
      if (!authUrl) delete callbacks[callbackName]
      container.replaceChildren()
    }
  }, [authUrl, callbackName, normalizedBotName])

  return (
    <div ref={containerRef} className={cn('flex justify-center', className)} />
  )
}
