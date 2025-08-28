// ============================================================================
// Core Kubernetes-like types (keep existing, add missing fields)
// ============================================================================

export interface ObjectMeta {
  name?: string;
  namespace?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  creationTimestamp?: string;
  uid?: string;
  resourceVersion?: string;
  generation?: number;
}

// ============================================================================
// Session types - align with existing but add OpenAPI completeness
// ============================================================================

export type IDE = "jupyterlab" | "vscode" | "rstudio" | "custom";

export interface ProfileSpec {
  ide: IDE;
  image: string;
  cmd?: string[];
}

export interface OIDCRef {
  issuerURL: string;
  clientIDSecret?: string;
  clientSecretRef?: string;
}

export type AuthMode = "oauth2proxy" | "none";

export interface AuthSpec {
  mode?: AuthMode;
  oidc?: OIDCRef;
}

export interface PVCSpec {
  size: string; // e.g. "10Gi"
  storageClassName?: string;
  mountPath: string;
}

export interface NetSpec {
  host?: string;
  tlsSecretName?: string;
  annotations?: Record<string, string>;
}

export interface SessionSpec {
  profile: ProfileSpec;
  auth?: AuthSpec;
  home?: PVCSpec;
  scratch?: PVCSpec;
  networking?: NetSpec;
  replicas?: number;
}

export type SessionPhase = "Pending" | "Ready" | "Error";

export interface SessionStatus {
  phase?: SessionPhase;
  url?: string;
  reason?: string;
}

// Main Session type - keep existing structure
export interface Session {
  apiVersion?: string;
  kind?: string;
  metadata: ObjectMeta;
  spec: SessionSpec;
  status?: SessionStatus;
}

// ============================================================================
// API Request/Response types - align with your existing patterns
// ============================================================================

export interface SessionCreateRequest {
  name: string;
  namespace?: string;
  profile: ProfileSpec;
  auth?: AuthSpec;
  home?: PVCSpec;
  scratch?: PVCSpec;
  networking?: NetSpec;
  replicas?: number;
}

export interface SessionListResponse {
  items: Session[];
  total: number;
  namespaces?: string[];
  filtered?: boolean;
}

export interface SessionScaleRequest {
  replicas: number;
}

export interface SessionDeleteResponse {
  status: string;
  name: string;
  namespace: string;
}

// ============================================================================
// Authentication types - keep existing but add OpenAPI fields
// ============================================================================

export interface AuthFeatures {
  ssoEnabled: boolean;
  localLoginEnabled: boolean;
  ssoLoginPath: string;
  localLoginPath: string;
}

export interface LocalLoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: string;
  roles: string[];
}

// ============================================================================
// User info types - enhance existing with OpenAPI completeness
// ============================================================================

export interface UserInfo {
  subject: string;
  username?: string;
  email?: string;
  roles: string[];
  provider?: string;
  issuedAt?: number;
  expiresAt?: number;
  implicitRoles?: string[];
}

// ============================================================================
// Introspection types - keep your existing structure but enhance
// ============================================================================

export interface PermissionCheck {
  resource: string;
  action: string;
  namespace: string;
  allowed: boolean;
}

export interface UserPermissions {
  subject: string;
  roles: string[];
  permissions: PermissionCheck[];
  namespaces: Record<string, string[]>; // namespace -> allowed actions
}

export interface DomainPermissions {
  session: Record<string, boolean>; // action -> allowed
}

export interface NamespaceInfo {
  userAllowed: string[];
  userCreatable?: string[];
  userDeletable?: string[];
}

export interface ServerNamespaceInfo {
  all?: string[];
  withSessions?: string[];
}

export interface UserCapabilities {
  namespaceScope: string[];
  clusterScope: boolean;
  adminAccess: boolean;
}

export interface SystemCapabilities {
  multiTenant: boolean;
}

export interface NamespacePermissions {
  list: boolean;
  watch: boolean;
}

export interface ServiceAccountInfo {
  namespaces: NamespacePermissions;
  session: Record<string, boolean>;
}

export interface CasbinPermissions {
  namespaces: NamespacePermissions;
}

export interface ClusterInfo {
  casbin: CasbinPermissions;
  serverServiceAccount: ServiceAccountInfo;
}

export interface ServerVersionInfo {
  version?: string;
  gitCommit?: string;
  buildDate?: string;
}

// Keep your existing introspection response types
export interface UserIntrospectionResponse {
  user: UserInfo;
  domains: Record<string, DomainPermissions>;
  namespaces: NamespaceInfo;
  capabilities: UserCapabilities;
}

export interface ServerIntrospectionResponse {
  cluster: ClusterInfo;
  namespaces: ServerNamespaceInfo;
  capabilities: SystemCapabilities;
  version?: ServerVersionInfo;
}

// Keep your legacy type for backward compatibility
export interface LegacyIntrospectionResponse {
  user: UserInfo;
  domains: Record<string, DomainPermissions>;
  namespaces: {
    all?: string[];
    withSessions?: string[];
    userAllowed: string[];
    userCreatable?: string[];
    userDeletable?: string[];
  };
  capabilities: {
    namespaceScope: string[];
    clusterScope: boolean;
    adminAccess: boolean;
    multiTenant: boolean;
  };
  cluster?: ClusterInfo;
}

// Maintain your existing aliases for compatibility
export type UserIntrospection = UserIntrospectionResponse;
export type ServerIntrospection = ServerIntrospectionResponse;
export type Introspection = LegacyIntrospectionResponse;

// ============================================================================
// Admin types
// ============================================================================

export interface AdminUserInfo {
  subject: string;
  roles: string[];
  active: boolean;
}

export interface AdminUsersResponse {
  users: AdminUserInfo[];
  total: number;
}

export interface RBACReloadResponse {
  status: string;
  message: string;
}

export interface SystemInfo {
  version: string;
  rbac: {
    modelPath: string;
    policyPath: string;
    status: string;
  };
  kubernetes: {
    gvr: {
      group: string;
      version: string;
      resource: string;
    };
  };
  authentication: {
    localLoginEnabled: boolean;
    oidcConfigured: boolean;
  };
}

// ============================================================================
// SSE and Event types - keep your existing
// ============================================================================

export type WatchEventType = "ADDED" | "MODIFIED" | "DELETED" | "ERROR";

export interface SessionWatchEvent {
  type: WatchEventType;
  object: Session;
}

export interface SSEMessage {
  event: "message" | "ping";
  data: SessionWatchEvent | { status?: string; timestamp?: string };
}

// Keep your existing SessionEvent alias
export type SessionEvent = SessionWatchEvent;

// ============================================================================
// Client configuration types - keep existing
// ============================================================================

export interface APIConfig {
  baseURL: string;
  timeout?: number;
  credentials?: RequestCredentials;
  defaultHeaders?: Record<string, string>;
}

export interface RequestOptions {
  method?: string;
  headers?: Record<string, string>;
  body?: any;
  signal?: AbortSignal;
}

export interface ListOptions {
  namespace?: string;
  all?: boolean;
}

export interface IntrospectionOptions {
  namespaces?: string[];
  actions?: string[];
  discover?: boolean;
}

export interface StreamOptions {
  namespace?: string;
  all?: boolean;
  onMessage?: (event: SessionWatchEvent) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
}

// ============================================================================
// Error handling - keep existing
// ============================================================================

export interface ErrorResponse {
  error: string;
}

export class CodespaceAPIError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public response?: any,
    message?: string,
  ) {
    super(message || `API Error: ${status} ${statusText}`);
    this.name = "CodespaceAPIError";
  }
}
