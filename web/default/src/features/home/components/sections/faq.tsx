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
import { Accordion as AccordionPrimitive } from '@base-ui/react/accordion'
import { Link } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Accordion,
  AccordionContent,
  AccordionItem,
} from '@/components/ui/accordion'

export function FAQ() {
  const { t } = useTranslation()
  const questions = [
    {
      id: 'balance',
      question: t('How do I top up my balance?'),
      answer: t(
        'Open your wallet in the dashboard, choose an amount and an available payment method, then follow the payment instructions.'
      ),
      to: '/wallet',
      link: t('Open wallet'),
    },
    {
      id: 'cost',
      question: t('How is the request cost calculated?'),
      answer: t(
        'Top up your platform quota and use it for model requests. Usage depends on the model price, token count and group multiplier.'
      ),
      to: '/pricing',
      link: t('View model pricing'),
    },
    {
      id: 'model',
      question: t('How do I choose a model?'),
      answer: t(
        'Compare models in the catalog by their capabilities, context window and price. Choose one that fits your task and use its model ID in your API request.'
      ),
      to: '/pricing',
      link: t('Explore models'),
    },
    {
      id: 'support',
      question: t('Where can I get help?'),
      answer: t(
        'Support contacts are listed on the About page. Include your client name, the error message and when it happened. Never share your password or API key.'
      ),
      to: '/about',
      link: t('Find support contacts'),
    },
  ] as const

  return (
    <section
      className='studio-connect studio-faq'
      aria-labelledby='home-faq-title'
    >
      <h2 id='home-faq-title'>{t('Frequently asked questions')}</h2>
      <Accordion className='studio-faq-list'>
        {questions.map((item) => (
          <AccordionItem
            key={item.id}
            value={item.id}
            className='studio-faq-item'
          >
            <AccordionPrimitive.Header>
              <AccordionPrimitive.Trigger className='studio-faq-trigger'>
                {item.question}
                <Plus
                  className='studio-faq-icon'
                  size={18}
                  aria-hidden='true'
                />
              </AccordionPrimitive.Trigger>
            </AccordionPrimitive.Header>
            <AccordionContent className='studio-faq-answer'>
              <p>{item.answer}</p>
              <Link to={item.to}>{item.link}</Link>
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    </section>
  )
}
