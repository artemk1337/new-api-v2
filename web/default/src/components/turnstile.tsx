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

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useRef } from 'react'

declare global {
  interface Window {
    turnstile?: {
      render: (element: HTMLElement, options: Record<string, unknown>) => string
      remove: (widgetId: string) => void
      reset: (widgetId: string) => void
    }
  }
}

interface TurnstileProps {
  siteKey: string
  action?: string
  onVerify: (token: string) => void
  onExpire?: () => void
  resetKey?: number
  className?: string
}

export function Turnstile({
  siteKey,
  action,
  onVerify,
  onExpire,
  resetKey = 0,
  className,
}: TurnstileProps) {
  const ref = useRef<HTMLDivElement | null>(null)
  const widgetIdRef = useRef<string | null>(null)
  const onVerifyRef = useRef(onVerify)
  const onExpireRef = useRef(onExpire)
  const previousResetKeyRef = useRef(resetKey)

  onVerifyRef.current = onVerify
  onExpireRef.current = onExpire

  useEffect(() => {
    const render = () => {
      if (!ref.current || !window.turnstile || widgetIdRef.current) return
      try {
        widgetIdRef.current = window.turnstile.render(ref.current, {
          sitekey: siteKey,
          ...(action ? { action } : {}),
          callback: (token: string) => onVerifyRef.current(token),
          'error-callback': () => onExpireRef.current?.(),
          'expired-callback': () => onExpireRef.current?.(),
        })
      } catch {
        /* empty */
      }
    }

    const scriptId = 'cf-turnstile'
    let scriptElement = document.querySelector(`#${scriptId}`)
    if (window.turnstile) {
      render()
    } else if (scriptElement) {
      scriptElement.addEventListener('load', render)
    } else {
      const script = document.createElement('script')
      script.id = scriptId
      script.src =
        'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
      script.async = true
      script.defer = true
      script.addEventListener('load', render)
      document.head.appendChild(script)
      scriptElement = script
    }

    return () => {
      scriptElement?.removeEventListener('load', render)
      if (widgetIdRef.current && window.turnstile) {
        window.turnstile.remove(widgetIdRef.current)
        widgetIdRef.current = null
      }
    }
  }, [siteKey, action])

  useEffect(() => {
    if (previousResetKeyRef.current === resetKey) return
    previousResetKeyRef.current = resetKey
    if (widgetIdRef.current && window.turnstile) {
      window.turnstile.reset(widgetIdRef.current)
    }
  }, [resetKey])

  return <div ref={ref} className={className} />
}
