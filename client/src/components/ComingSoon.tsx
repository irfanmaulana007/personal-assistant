interface ComingSoonProps {
  title: string;
  description: string;
}

export function ComingSoon({ title, description }: ComingSoonProps) {
  return (
    <div className="flex-1 flex items-center justify-center p-6">
      <div className="max-w-sm text-center">
        <div className="mx-auto mb-4 w-12 h-12 bg-indigo-100 rounded-2xl flex items-center justify-center">
          <svg
            className="w-6 h-6 text-indigo-600"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
        </div>
        <h2 className="text-lg font-semibold text-gray-900">{title}</h2>
        <p className="mt-1 text-sm text-gray-500">{description}</p>
        <span className="mt-4 inline-block rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-500">
          Coming soon
        </span>
      </div>
    </div>
  );
}
