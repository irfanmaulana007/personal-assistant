// The domain/API types now live in the shared package so the web app and the
// future React Native app share one source of truth. Re-exported here so the
// existing `../types` import paths across the web app keep working unchanged.
export * from '@personal-assistant/shared/types';
