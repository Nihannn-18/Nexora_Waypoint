'use client';

import { useEffect, useId, useRef, type ReactNode } from 'react';
import { buttonClass } from './ui';

/**
 * A deliberate confirmation step. While `busy`, both buttons are disabled and
 * Escape is ignored, so a request in flight can be neither doubled nor
 * abandoned half-way from the UI.
 */
export function ConfirmationDialog({
  open,
  title,
  eyebrow,
  children,
  confirmLabel,
  busyLabel,
  busy,
  confirmDisabled = false,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  eyebrow?: string;
  children: ReactNode;
  confirmLabel: string;
  busyLabel: string;
  busy: boolean;
  confirmDisabled?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const titleId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    // Focus the safe choice first: confirming should take a second action.
    cancelRef.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, busy, onCancel]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-ink/40 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex max-h-[90dvh] w-full max-w-xl flex-col rounded-card bg-card shadow-xl ring-1 ring-ink/10"
      >
        <div className="border-b border-ink/10 px-5 py-4">
          {eyebrow && (
            <p className="font-mono text-[0.6875rem] font-semibold uppercase tracking-wider text-warning">
              {eyebrow}
            </p>
          )}
          <h2 id={titleId} className="mt-0.5 text-lg font-semibold text-ink">
            {title}
          </h2>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4 text-sm text-ink">
          {children}
        </div>
        <div className="flex flex-wrap justify-end gap-2 border-t border-ink/10 px-5 py-3">
          <button
            ref={cancelRef}
            type="button"
            className={buttonClass.secondary}
            onClick={onCancel}
            disabled={busy}
          >
            Cancel
          </button>
          <button
            type="button"
            className={buttonClass.primary}
            onClick={onConfirm}
            disabled={busy || confirmDisabled}
            aria-busy={busy}
          >
            {busy ? busyLabel : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
