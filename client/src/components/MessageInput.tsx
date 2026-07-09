import { useState, useEffect, useRef, type FormEvent, type KeyboardEvent } from 'react';

interface MessageInputProps {
  onSend: (message: string, image?: string) => void;
  disabled: boolean;
}

const MAX_IMAGE_BYTES = 5 * 1024 * 1024; // 5 MB

export function MessageInput({ onSend, disabled }: MessageInputProps) {
  const [text, setText] = useState('');
  const [image, setImage] = useState<string | null>(null);
  const [imageError, setImageError] = useState('');
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  // Focus the input on mount (i.e. every time the Chat page is opened) and
  // again whenever the assistant finishes replying.
  useEffect(() => {
    if (!disabled) inputRef.current?.focus();
  }, [disabled]);

  const canSend = (text.trim() !== '' || image !== null) && !disabled;

  const submit = () => {
    if (!canSend) return;
    onSend(text.trim(), image ?? undefined);
    setText('');
    setImage(null);
    setImageError('');
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit();
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  };

  const pickImage = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ''; // allow re-selecting the same file
    if (!file) return;
    if (!file.type.startsWith('image/')) {
      setImageError('Please choose an image file.');
      return;
    }
    if (file.size > MAX_IMAGE_BYTES) {
      setImageError('Image is too large (max 5 MB).');
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      setImage(typeof reader.result === 'string' ? reader.result : null);
      setImageError('');
    };
    reader.readAsDataURL(file);
  };

  return (
    <form onSubmit={handleSubmit} className="border-t border-gray-200 bg-white p-4">
      <div>
        {image && (
          <div className="mb-2 flex items-center gap-2">
            <div className="relative">
              <img src={image} alt="attachment" className="h-16 w-16 rounded-lg object-cover" />
              <button
                type="button"
                onClick={() => setImage(null)}
                aria-label="Remove image"
                className="absolute -right-1.5 -top-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-gray-800 text-xs text-white shadow"
              >
                ×
              </button>
            </div>
          </div>
        )}
        {imageError && <p className="mb-2 text-xs text-red-600">{imageError}</p>}

        <div className="flex items-end gap-2">
          <input
            ref={fileRef}
            type="file"
            accept="image/*"
            onChange={pickImage}
            className="hidden"
          />
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            disabled={disabled}
            aria-label="Attach image"
            title="Attach an image"
            className="rounded-xl border border-gray-200 p-2.5 text-gray-500 transition hover:bg-gray-50 hover:text-gray-900 disabled:opacity-50"
          >
            <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M15.172 7l-6.586 6.586a2 2 0 102.828 2.828l6.414-6.586a4 4 0 00-5.656-5.656l-6.415 6.585a6 6 0 108.486 8.486L20.5 13"
              />
            </svg>
          </button>
          <textarea
            ref={inputRef}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message..."
            rows={1}
            disabled={disabled}
            className="flex-1 resize-none rounded-xl border border-gray-200 px-4 py-2.5 text-sm text-gray-900 outline-none transition placeholder:text-gray-400 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 disabled:opacity-50"
          />
          <button
            type="submit"
            disabled={!canSend}
            className="rounded-xl bg-indigo-600 px-4 py-2.5 text-white transition hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"
              />
            </svg>
          </button>
        </div>
      </div>
    </form>
  );
}
