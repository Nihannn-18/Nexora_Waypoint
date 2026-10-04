'use client';

import { useState, type InputHTMLAttributes } from 'react';
import { EyeIcon, EyeOffIcon } from './icons';

/**
 * A password field with a show/hide toggle. The toggle is a real button
 * (type="button", so Enter still submits the form) with a label that names the
 * action, and a 48px tap target inside the field's right edge.
 */
export function PasswordInput({
  id,
  className = '',
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & { id: string }) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <input
        id={id}
        type={visible ? 'text' : 'password'}
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        className={`tap-target w-full rounded-control bg-card pl-3 pr-12 text-ink ring-1 ring-ink/15 focus-visible:ring-2 focus-visible:ring-brand ${className}`}
        {...props}
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        aria-label={visible ? 'Hide password' : 'Show password'}
        aria-controls={id}
        className="tap-target absolute inset-y-0 right-0 grid place-items-center rounded-control px-3 text-ink-muted hover:text-ink focus-visible:ring-2 focus-visible:ring-brand"
      >
        {visible ? (
          <EyeOffIcon width={20} height={20} />
        ) : (
          <EyeIcon width={20} height={20} />
        )}
      </button>
    </div>
  );
}
