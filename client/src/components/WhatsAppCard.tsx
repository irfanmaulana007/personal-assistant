import { useState, useEffect } from 'react';
import { getWhatsApp, connectWhatsApp, disconnectWhatsApp } from '../api/client';
import type { WhatsAppStatus } from '../types';

const badge: Record<string, { label: string; cls: string }> = {
  connected: {
    label: 'Connected',
    cls: 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300',
  },
  pairing: {
    label: 'Waiting for scan',
    cls: 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  },
  disconnected: {
    label: 'Not connected',
    cls: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
  },
  disabled: {
    label: 'Disabled',
    cls: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
  },
};

export function WhatsAppCard() {
  const [wa, setWa] = useState<WhatsAppStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');

  useEffect(() => {
    let active = true;
    getWhatsApp()
      .then((d) => {
        if (active) setWa(d);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, []);

  // Poll while pairing so the card updates once the QR is scanned.
  useEffect(() => {
    if (wa?.status !== 'pairing') return;
    const id = setInterval(() => {
      getWhatsApp()
        .then(setWa)
        .catch(() => {});
    }, 2500);
    return () => clearInterval(id);
  }, [wa?.status]);

  if (!wa || !wa.enabled) return null;

  const connect = async () => {
    setBusy(true);
    setErr('');
    try {
      setWa(await connectWhatsApp());
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not start pairing');
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    setBusy(true);
    setErr('');
    try {
      setWa(await disconnectWhatsApp());
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not disconnect');
    } finally {
      setBusy(false);
    }
  };

  const status = badge[wa.status] ?? badge.disconnected;

  return (
    <div className="mt-6 rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-green-100 dark:bg-green-500/15 text-sm font-semibold text-green-700 dark:text-green-300">
            WA
          </div>
          <div>
            <div className="text-sm font-semibold text-gray-900 dark:text-gray-50">WhatsApp</div>
            <span
              className={`mt-0.5 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${status.cls}`}
            >
              {status.label}
            </span>
          </div>
        </div>
        {wa.status === 'connected' ? (
          <button
            onClick={disconnect}
            disabled={busy}
            className="rounded-xl border border-gray-200 dark:border-gray-700 px-3 py-2 text-sm font-medium text-red-600 dark:text-red-400 transition hover:bg-red-50 dark:hover:bg-red-500/15 disabled:opacity-50"
          >
            Disconnect
          </button>
        ) : wa.status === 'pairing' ? (
          <button
            onClick={disconnect}
            disabled={busy}
            className="rounded-xl border border-gray-200 dark:border-gray-700 px-3 py-2 text-sm font-medium text-gray-600 dark:text-gray-300 transition hover:bg-gray-50 dark:hover:bg-gray-800/60 disabled:opacity-50"
          >
            Cancel
          </button>
        ) : (
          <button
            onClick={connect}
            disabled={busy}
            className="rounded-xl bg-indigo-600 dark:bg-indigo-500 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-700 dark:hover:bg-indigo-600 disabled:opacity-50"
          >
            {busy ? 'Starting…' : 'Connect'}
          </button>
        )}
      </div>

      {wa.status === 'disconnected' && (
        <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
          Link your personal WhatsApp so you can chat with the assistant from WhatsApp.
        </p>
      )}

      {wa.status === 'pairing' && (
        <div className="mt-4 flex flex-col items-center gap-3 rounded-xl border border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-900 p-4 text-center">
          {wa.qr ? (
            <img src={wa.qr} alt="WhatsApp QR code" className="h-56 w-56 rounded-lg bg-white p-2" />
          ) : (
            <p className="text-sm text-gray-500 dark:text-gray-400">Generating QR code…</p>
          )}
          <p className="max-w-xs text-xs text-gray-500 dark:text-gray-400">
            On your phone, open{' '}
            <span className="font-medium">
              WhatsApp → Settings → Linked Devices → Link a Device
            </span>{' '}
            and scan this code. It refreshes automatically.
          </p>
        </div>
      )}

      {err && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{err}</p>}
    </div>
  );
}
