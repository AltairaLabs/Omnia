/**
 * Audit logging module.
 *
 * @module audit
 */

export {
  logAudit,
  logCrdSuccess,
  logCrdDenied,
  logCrdError,
  logWarn,
  logError,
  createAuditLogger,
  methodToAction,
  isAuditLoggingEnabled,
  type AuditEntry,
  type AuditInput,
  type AuditAction,
  type AuditResult,
} from "./logger";
