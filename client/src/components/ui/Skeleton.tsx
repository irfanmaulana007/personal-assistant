// Skeleton loaders — shimmering placeholders shown while a page's data loads.
// Compose the `Skeleton` primitive into layout-matching shapes so the loading
// state mirrors the real content and avoids a jarring layout shift on arrival.

/** A single shimmering placeholder block. Size it with Tailwind classes. */
export function Skeleton({ className = '' }: { className?: string }) {
  return (
    <div
      aria-hidden
      className={`animate-pulse rounded-md bg-gray-200 dark:bg-gray-700 ${className}`}
    />
  );
}

/** A rounded card matching the app's card chrome, filled with skeleton content. */
export function SkeletonCard({
  className = '',
  children,
}: {
  className?: string;
  children?: React.ReactNode;
}) {
  return (
    <div
      className={`rounded-2xl border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800 ${className}`}
    >
      {children}
    </div>
  );
}

/** A stat-tile placeholder: small label line above a large value line. */
export function SkeletonStatTile() {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <Skeleton className="h-2.5 w-16" />
      <Skeleton className="mt-2.5 h-6 w-24" />
    </div>
  );
}

/**
 * A form-card placeholder: heading + subtext, a stack of labelled input rows,
 * and a submit button. Used by the settings sub-pages, which each render a
 * single `p-6` form card once loaded.
 */
export function SkeletonFormCard({ fields = 4 }: { fields?: number }) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
      <Skeleton className="h-4 w-40" />
      <Skeleton className="mt-2 h-3 w-72 max-w-full" />
      <div className="mt-5 space-y-4">
        {Array.from({ length: fields }).map((_, i) => (
          <div key={i}>
            <Skeleton className="mb-1.5 h-3 w-24" />
            <Skeleton className="h-10 w-full rounded-xl" />
          </div>
        ))}
      </div>
      <Skeleton className="mt-5 h-10 w-24 rounded-xl" />
    </div>
  );
}

/**
 * A list-row placeholder used by pages that render a stack of cards, each with
 * a title + description on the left and a control on the right.
 */
export function SkeletonListRow({ trailingWidth = 'w-11' }: { trailingWidth?: string }) {
  return (
    <div className="flex items-start gap-4 rounded-2xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div className="min-w-0 flex-1">
        <Skeleton className="h-3.5 w-40" />
        <Skeleton className="mt-2 h-3 w-64 max-w-full" />
      </div>
      <Skeleton className={`h-6 ${trailingWidth} shrink-0 rounded-full`} />
    </div>
  );
}
