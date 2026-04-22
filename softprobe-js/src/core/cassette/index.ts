/**
 * Shared cassette interfaces and adapters.
 */
export { CaseJsonFileCassette } from './case-json-file-cassette';
export { buildCaseDocumentFromRecords, caseDocumentToCassetteRecords } from './case-bridge';
/** @deprecated NDJSON cassette removed from the default path; kept for internal tooling migration only. */
export { NdjsonCassette } from './ndjson-cassette';
export { resolveRequestStorage } from './request-storage';
export { resolveRequestStorageForContext } from './context-request-storage';
export { getCaptureStore, setCaptureStore } from './capture-store-accessor';
