// All entity types are inferred from Zod schemas in schemas.ts.
// Import types from here as before — these re-exports preserve backward compatibility.
export type {
  AccountInfo,
  AnalyticsData,
  BillingStatus,
  BlockedTerm,
  Comment,
  ImportResult,
  SecuritySettings,
  Site,
  TableInfo,
  TeamMember,
  User,
} from './schemas'

// ColumnMapping is a request body type (constructed in the UI, never parsed
// from a server response), so it stays as a plain interface here.
export interface ColumnMapping {
  table: string
  columns: Record<string, string>
  strip_domain: boolean
  wrap_in_p: boolean
}
