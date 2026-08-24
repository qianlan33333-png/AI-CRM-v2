import {
  addCustomerTag,
  deleteCustomerContactPolicy,
  getCustomer,
  getCustomerContactPolicy,
  listCustomerEvents,
  listTags,
  putCustomerContactPolicy,
  removeCustomerTag,
  setCustomerStage,
  updateCustomer,
  type ClearCustomerContactPolicyRequest,
  type SetCustomerContactPolicyRequest,
  type CustomerUpdateRequest,
  type ListCustomerEventsParams,
  type SetCustomerStageRequest,
} from "./api/generated/health";

export interface CustomerProfile {
  readonly id: number;
  readonly name: string;
  readonly avatarURL?: string;
  readonly gender?: number;
  readonly stageID?: number;
  readonly ownerStaffID?: number;
  readonly channelID?: number;
  readonly addedAt?: string;
  readonly lastInteractAt?: string;
  readonly isDeleted: boolean;
  readonly createdAt: string;
  readonly updatedAt: string;
}

export interface CustomerTag {
  readonly id: number;
  readonly groupName?: string;
  readonly name: string;
  readonly sortOrder: number;
}

export interface CustomerTimelineEvent {
  readonly id: number;
  readonly eventType: string;
  readonly actor: string;
  readonly occurredAt: string;
}

export type CustomerContactPolicyReason =
  "manual_opt_out" | "compliance_hold" | "operator_hold";

export interface CustomerContactPolicy {
  readonly customerID: number;
  readonly version: number;
  readonly policyPresent: boolean;
  readonly eligible: boolean;
  readonly suppressionActive: boolean;
  readonly reasonCode: CustomerContactPolicyReason | null;
  readonly suppressedUntil: string | null;
  readonly localOnly: true;
}

export interface CustomerDetailSnapshot {
  readonly customer: CustomerProfile;
  readonly tags: readonly CustomerTag[];
  readonly tagCatalog: readonly CustomerTag[];
  readonly events: readonly CustomerTimelineEvent[];
  readonly eventsHaveMore: boolean;
  readonly eventsNextCursor?: string;
}

export interface CustomerProfileUpdate {
  readonly name: string;
  readonly avatarURL: string | null;
  readonly gender: number | null;
  readonly ownerStaffID: number | null;
  readonly channelID: number | null;
}

export interface CustomerDetailTransportResponse {
  readonly status: number;
  readonly data: unknown;
}

async function loadGeneratedCustomer(
  customerID: number,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await getCustomer(customerID, options);
  return { status: response.status, data: response.data };
}

async function updateGeneratedCustomer(
  customerID: number,
  request: CustomerUpdateRequest,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await updateCustomer(customerID, request, options);
  return { status: response.status, data: response.data };
}

async function setGeneratedCustomerStage(
  customerID: number,
  request: SetCustomerStageRequest,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await setCustomerStage(customerID, request, options);
  return { status: response.status, data: response.data };
}

async function addGeneratedCustomerTag(
  customerID: number,
  tagID: number,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await addCustomerTag(customerID, tagID, options);
  return { status: response.status, data: response.data };
}

async function removeGeneratedCustomerTag(
  customerID: number,
  tagID: number,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await removeCustomerTag(customerID, tagID, options);
  return { status: response.status, data: response.data };
}

async function loadGeneratedCustomerEvents(
  customerID: number,
  params: ListCustomerEventsParams,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await listCustomerEvents(customerID, params, options);
  return { status: response.status, data: response.data };
}

async function loadGeneratedTags(
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await listTags(options);
  return { status: response.status, data: response.data };
}

async function loadGeneratedCustomerContactPolicy(
  customerID: number,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await getCustomerContactPolicy(customerID, options);
  return { status: response.status, data: response.data };
}

async function setGeneratedCustomerContactPolicy(
  customerID: number,
  request: SetCustomerContactPolicyRequest,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await putCustomerContactPolicy(customerID, request, options);
  return { status: response.status, data: response.data };
}

async function clearGeneratedCustomerContactPolicy(
  customerID: number,
  request: ClearCustomerContactPolicyRequest,
  options: RequestInit,
): Promise<CustomerDetailTransportResponse> {
  const response = await deleteCustomerContactPolicy(
    customerID,
    request,
    options,
  );
  return { status: response.status, data: response.data };
}

export interface CustomerDetailTransport {
  readonly get: typeof loadGeneratedCustomer;
  readonly update: typeof updateGeneratedCustomer;
  readonly setStage: typeof setGeneratedCustomerStage;
  readonly addTag: typeof addGeneratedCustomerTag;
  readonly removeTag: typeof removeGeneratedCustomerTag;
  readonly listEvents: typeof loadGeneratedCustomerEvents;
  readonly listTags: typeof loadGeneratedTags;
  readonly getContactPolicy: typeof loadGeneratedCustomerContactPolicy;
  readonly setContactPolicy: typeof setGeneratedCustomerContactPolicy;
  readonly clearContactPolicy: typeof clearGeneratedCustomerContactPolicy;
}

export const generatedCustomerDetailTransport: CustomerDetailTransport = {
  get: loadGeneratedCustomer,
  update: updateGeneratedCustomer,
  setStage: setGeneratedCustomerStage,
  addTag: addGeneratedCustomerTag,
  removeTag: removeGeneratedCustomerTag,
  listEvents: loadGeneratedCustomerEvents,
  listTags: loadGeneratedTags,
  getContactPolicy: loadGeneratedCustomerContactPolicy,
  setContactPolicy: setGeneratedCustomerContactPolicy,
  clearContactPolicy: clearGeneratedCustomerContactPolicy,
};

export type CustomerDetailLoadResult =
  | { readonly status: "loaded"; readonly snapshot: CustomerDetailSnapshot }
  | { readonly status: "unauthenticated" }
  | { readonly status: "forbidden" }
  | { readonly status: "not_found" }
  | { readonly status: "unavailable" };

type CustomerDetailReadFailure = Exclude<
  CustomerDetailLoadResult,
  { readonly status: "loaded" }
>;

export type CustomerTimelineLoadResult =
  | {
      readonly status: "loaded";
      readonly events: readonly CustomerTimelineEvent[];
      readonly nextCursor?: string;
    }
  | CustomerDetailReadFailure;

export type CustomerMutationFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

export type CustomerContactPolicyLoadResult =
  | { readonly status: "loaded"; readonly policy: CustomerContactPolicy }
  | { readonly status: "unauthenticated" }
  | { readonly status: "forbidden" }
  | { readonly status: "not_found" }
  | { readonly status: "unavailable" };

export type CustomerContactPolicyMutationResult =
  | { readonly status: "succeeded"; readonly policy: CustomerContactPolicy }
  | { readonly status: "unauthenticated" }
  | { readonly status: "forbidden" }
  | { readonly status: "not_found" }
  | { readonly status: "conflict" }
  | { readonly status: "invalid" }
  | { readonly status: "unavailable" }
  | { readonly status: "outcome_unknown" };

export type CustomerMutationResult =
  | { readonly status: "succeeded" }
  | { readonly status: CustomerMutationFailure };

const SAME_ORIGIN: RequestInit = { credentials: "same-origin" };
const EVENT_PAGE_SIZE = 50;
const CSRF_TOKEN_PATTERN = /^[A-Za-z0-9_-]{43}$/;
const CUSTOMER_GENDER_MIN = -32_768;
const CUSTOMER_GENDER_MAX = 32_767;
const RFC3339_TIMESTAMP_PATTERN =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

function plainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function positiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function safeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value);
}

export function isCustomerGender(value: unknown): value is number {
  return (
    safeInteger(value) &&
    value >= CUSTOMER_GENDER_MIN &&
    value <= CUSTOMER_GENDER_MAX
  );
}

export function isSafeAvatarURL(value: unknown): value is string {
  if (
    typeof value !== "string" ||
    value.length === 0 ||
    value !== value.trim() ||
    !/^https?:\/\//i.test(value)
  ) {
    return false;
  }
  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.hostname.length > 0 &&
      parsed.username === "" &&
      parsed.password === ""
    );
  } catch {
    return false;
  }
}

function timestamp(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    RFC3339_TIMESTAMP_PATTERN.test(value) &&
    Number.isFinite(Date.parse(value))
  );
}

function optionalTimestamp(value: unknown): value is string | null | undefined {
  return value === undefined || value === null || timestamp(value);
}

function optionalPositiveInteger(
  value: unknown,
): value is number | null | undefined {
  return value === undefined || value === null || positiveInteger(value);
}

function optionalCustomerGender(
  value: unknown,
): value is number | null | undefined {
  return value === undefined || value === null || isCustomerGender(value);
}

function optionalAvatarURL(value: unknown): value is string | null | undefined {
  return value === undefined || value === null || isSafeAvatarURL(value);
}

function optionalString(value: unknown): value is string | null | undefined {
  return value === undefined || value === null || typeof value === "string";
}

function nonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function exactObject(
  value: unknown,
  required: readonly string[],
  allowed: readonly string[],
): value is Record<string, unknown> {
  if (!plainRecord(value)) return false;
  const keys = Object.keys(value);
  return (
    required.every((key) => Object.hasOwn(value, key)) &&
    keys.every((key) => allowed.includes(key))
  );
}

function parseCustomer(value: unknown): CustomerProfile | undefined {
  const required = [
    "id",
    "name",
    "is_deleted",
    "extra",
    "created_at",
    "updated_at",
  ];
  const allowed = [
    ...required,
    "avatar_url",
    "gender",
    "stage_id",
    "owner_staff_id",
    "channel_id",
    "added_at",
    "last_interact_at",
  ];
  if (!exactObject(value, required, allowed)) return undefined;
  if (
    !positiveInteger(value.id) ||
    typeof value.name !== "string" ||
    !plainRecord(value.extra) ||
    typeof value.is_deleted !== "boolean" ||
    !timestamp(value.created_at) ||
    !timestamp(value.updated_at) ||
    !optionalAvatarURL(value.avatar_url) ||
    !optionalCustomerGender(value.gender) ||
    !optionalPositiveInteger(value.stage_id) ||
    !optionalPositiveInteger(value.owner_staff_id) ||
    !optionalPositiveInteger(value.channel_id) ||
    !optionalTimestamp(value.added_at) ||
    !optionalTimestamp(value.last_interact_at)
  ) {
    return undefined;
  }
  if (Date.parse(value.created_at) > Date.parse(value.updated_at)) {
    return undefined;
  }

  return {
    id: value.id,
    name: value.name,
    ...(typeof value.avatar_url === "string"
      ? { avatarURL: value.avatar_url }
      : {}),
    ...(typeof value.gender === "number" ? { gender: value.gender } : {}),
    ...(typeof value.stage_id === "number" ? { stageID: value.stage_id } : {}),
    ...(typeof value.owner_staff_id === "number"
      ? { ownerStaffID: value.owner_staff_id }
      : {}),
    ...(typeof value.channel_id === "number"
      ? { channelID: value.channel_id }
      : {}),
    ...(typeof value.added_at === "string" ? { addedAt: value.added_at } : {}),
    ...(typeof value.last_interact_at === "string"
      ? { lastInteractAt: value.last_interact_at }
      : {}),
    isDeleted: value.is_deleted,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  };
}

function parseTag(value: unknown): CustomerTag | undefined {
  const required = ["id", "name", "sort_order"];
  const allowed = [...required, "group_id", "group_name"];
  if (!exactObject(value, required, allowed)) return undefined;
  if (
    !positiveInteger(value.id) ||
    typeof value.name !== "string" ||
    value.name.trim().length === 0 ||
    value.name.length > 200 ||
    !safeInteger(value.sort_order) ||
    value.sort_order < -2_147_483_648 ||
    value.sort_order > 2_147_483_647 ||
    !optionalPositiveInteger(value.group_id) ||
    !optionalString(value.group_name)
  ) {
    return undefined;
  }
  const hasGroupID = typeof value.group_id === "number";
  const hasGroupName = typeof value.group_name === "string";
  if (
    hasGroupID !== hasGroupName ||
    (typeof value.group_name === "string" &&
      value.group_name.trim().length === 0)
  ) {
    return undefined;
  }

  return {
    id: value.id,
    name: value.name,
    sortOrder: value.sort_order,
    ...(typeof value.group_name === "string"
      ? { groupName: value.group_name }
      : {}),
  };
}

function parseTags(value: unknown): readonly CustomerTag[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const tags: CustomerTag[] = [];
  const seen = new Set<number>();
  for (const item of value) {
    const tag = parseTag(item);
    if (!tag || seen.has(tag.id)) return undefined;
    seen.add(tag.id);
    tags.push(tag);
  }
  return tags;
}

function parseEvent(value: unknown): CustomerTimelineEvent | undefined {
  const required = [
    "id",
    "customer_id",
    "event_type",
    "payload",
    "actor",
    "occurred_at",
  ];
  if (!exactObject(value, required, required)) return undefined;
  if (
    !positiveInteger(value.id) ||
    !positiveInteger(value.customer_id) ||
    typeof value.event_type !== "string" ||
    value.event_type.length === 0 ||
    !plainRecord(value.payload) ||
    typeof value.actor !== "string" ||
    value.actor.length === 0 ||
    value.actor.length > 200 ||
    !timestamp(value.occurred_at)
  ) {
    return undefined;
  }

  return {
    id: value.id,
    eventType: value.event_type,
    actor: value.actor,
    occurredAt: value.occurred_at,
  };
}

function parseEventPage(
  value: unknown,
  customerID: number,
):
  | {
      readonly events: readonly CustomerTimelineEvent[];
      readonly nextCursor?: string;
    }
  | undefined {
  if (!exactObject(value, ["items", "next_cursor"], ["items", "next_cursor"])) {
    return undefined;
  }
  if (!Array.isArray(value.items)) return undefined;
  if (
    value.next_cursor !== null &&
    (typeof value.next_cursor !== "string" ||
      value.next_cursor.length === 0 ||
      value.next_cursor.length > 512)
  ) {
    return undefined;
  }

  const events: CustomerTimelineEvent[] = [];
  const seen = new Set<number>();
  for (const item of value.items) {
    const event = parseEvent(item);
    if (!event || seen.has(event.id)) return undefined;
    if ((item as Record<string, unknown>).customer_id !== customerID) {
      return undefined;
    }
    seen.add(event.id);
    events.push(event);
  }
  return {
    events,
    ...(typeof value.next_cursor === "string"
      ? { nextCursor: value.next_cursor }
      : {}),
  };
}

export function parseCustomerDetailResponse(
  value: unknown,
  customerID: number,
):
  | {
      readonly customer: CustomerProfile;
      readonly tags: readonly CustomerTag[];
    }
  | undefined {
  if (!positiveInteger(customerID)) return undefined;
  if (!exactObject(value, ["customer", "tags"], ["customer", "tags"])) {
    return undefined;
  }
  const customer = parseCustomer(value.customer);
  const tags = parseTags(value.tags);
  if (!customer || customer.id !== customerID || !tags) return undefined;
  return { customer, tags };
}

export function parseTagCatalog(
  value: unknown,
): readonly CustomerTag[] | undefined {
  if (!exactObject(value, ["items"], ["items"])) return undefined;
  return parseTags(value.items);
}

const contactPolicyReasons = new Set<CustomerContactPolicyReason>([
  "manual_opt_out",
  "compliance_hold",
  "operator_hold",
]);

function contactPolicyReason(
  value: unknown,
): value is CustomerContactPolicyReason {
  return (
    typeof value === "string" &&
    contactPolicyReasons.has(value as CustomerContactPolicyReason)
  );
}

export function parseCustomerContactPolicy(
  value: unknown,
  customerID: number,
): CustomerContactPolicy | undefined {
  const keys = [
    "customer_id",
    "version",
    "policy_present",
    "eligible",
    "suppression_active",
    "reason_code",
    "suppressed_until",
    "local_only",
    "provider_execution_eligible",
    "real_external_call_executed",
    "delivery_proven",
  ];
  if (!positiveInteger(customerID) || !exactObject(value, keys, keys)) {
    return undefined;
  }
  if (
    value.customer_id !== customerID ||
    !nonNegativeInteger(value.version) ||
    typeof value.policy_present !== "boolean" ||
    typeof value.eligible !== "boolean" ||
    typeof value.suppression_active !== "boolean" ||
    !(value.reason_code === null || contactPolicyReason(value.reason_code)) ||
    !(value.suppressed_until === null || timestamp(value.suppressed_until)) ||
    value.local_only !== true ||
    value.provider_execution_eligible !== false ||
    value.real_external_call_executed !== false ||
    value.delivery_proven !== false ||
    value.eligible === value.suppression_active
  ) {
    return undefined;
  }
  if (!value.policy_present) {
    if (
      value.version !== 0 ||
      value.reason_code !== null ||
      value.suppressed_until !== null ||
      !value.eligible ||
      value.suppression_active
    ) {
      return undefined;
    }
  } else if (value.version < 1 || !contactPolicyReason(value.reason_code)) {
    return undefined;
  }
  return {
    customerID,
    version: value.version,
    policyPresent: value.policy_present,
    eligible: value.eligible,
    suppressionActive: value.suppression_active,
    reasonCode: value.reason_code,
    suppressedUntil: value.suppressed_until,
    localOnly: true,
  };
}

export async function loadCustomerContactPolicy(
  transport: CustomerDetailTransport,
  customerID: number,
  signal?: AbortSignal,
): Promise<CustomerContactPolicyLoadResult> {
  if (!positiveInteger(customerID)) return { status: "unavailable" };
  try {
    const response = await transport.getContactPolicy(customerID, {
      ...SAME_ORIGIN,
      ...(signal ? { signal } : {}),
    });
    if (response.status === 401) return { status: "unauthenticated" };
    if (response.status === 403) return { status: "forbidden" };
    if (response.status === 404) return { status: "not_found" };
    if (response.status !== 200) return { status: "unavailable" };
    const policy = parseCustomerContactPolicy(response.data, customerID);
    return policy ? { status: "loaded", policy } : { status: "unavailable" };
  } catch {
    return { status: "unavailable" };
  }
}

function readFailure(
  statuses: readonly number[],
): CustomerDetailReadFailure | undefined {
  if (statuses.includes(401)) return { status: "unauthenticated" };
  if (statuses.includes(404)) return { status: "not_found" };
  if (statuses.includes(403)) return { status: "forbidden" };
  if (statuses.some((status) => status !== 200)) {
    return { status: "unavailable" };
  }
  return undefined;
}

export async function loadCustomerDetail(
  transport: CustomerDetailTransport,
  customerID: number,
): Promise<CustomerDetailLoadResult> {
  if (!positiveInteger(customerID)) return { status: "unavailable" };

  try {
    const [detail, events, catalog] = await Promise.all([
      transport.get(customerID, SAME_ORIGIN),
      transport.listEvents(customerID, { limit: EVENT_PAGE_SIZE }, SAME_ORIGIN),
      transport.listTags(SAME_ORIGIN),
    ]);
    const failure = readFailure([detail.status, events.status, catalog.status]);
    if (failure) return failure;

    const parsedDetail = parseCustomerDetailResponse(detail.data, customerID);
    const parsedEvents = parseEventPage(events.data, customerID);
    const parsedCatalog = parseTagCatalog(catalog.data);
    if (!parsedDetail || !parsedEvents || !parsedCatalog) {
      return { status: "unavailable" };
    }
    return {
      status: "loaded",
      snapshot: {
        ...parsedDetail,
        tagCatalog: parsedCatalog,
        events: parsedEvents.events,
        eventsHaveMore: parsedEvents.nextCursor !== undefined,
        ...(parsedEvents.nextCursor !== undefined
          ? { eventsNextCursor: parsedEvents.nextCursor }
          : {}),
      },
    };
  } catch {
    return { status: "unavailable" };
  }
}

function validKnownEventIDs(value: ReadonlySet<number>): boolean {
  for (const id of value) {
    if (!positiveInteger(id)) return false;
  }
  return true;
}

export async function loadCustomerTimelinePage(
  transport: CustomerDetailTransport,
  customerID: number,
  cursor: string,
  knownEventIDs: ReadonlySet<number>,
): Promise<CustomerTimelineLoadResult> {
  if (
    !positiveInteger(customerID) ||
    typeof cursor !== "string" ||
    cursor.length === 0 ||
    cursor.length > 512 ||
    !validKnownEventIDs(knownEventIDs)
  ) {
    return { status: "unavailable" };
  }

  try {
    const response = await transport.listEvents(
      customerID,
      { cursor, limit: EVENT_PAGE_SIZE },
      SAME_ORIGIN,
    );
    const failure = readFailure([response.status]);
    if (failure) return failure;
    const parsed = parseEventPage(response.data, customerID);
    if (
      parsed === undefined ||
      parsed.events.some((event) => knownEventIDs.has(event.id))
    ) {
      return { status: "unavailable" };
    }
    return { status: "loaded", ...parsed };
  } catch {
    return { status: "unavailable" };
  }
}

function mutationFailure(status: number): CustomerMutationFailure {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 400 || status === 422) return "invalid";
  return "unavailable";
}

function mutationOptions(csrfToken: string): RequestInit | undefined {
  if (!CSRF_TOKEN_PATTERN.test(csrfToken)) return undefined;
  return {
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrfToken },
  };
}

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export function newCustomerContactPolicyIdempotencyKey(
  operation: "set" | "clear",
  source: { readonly randomUUID: () => string } | undefined = globalThis.crypto,
): string | undefined {
  try {
    const uuid = source?.randomUUID();
    return typeof uuid === "string" && UUID_PATTERN.test(uuid)
      ? `customer-contact-policy:${operation}:${uuid}`
      : undefined;
  } catch {
    return undefined;
  }
}

function contactPolicyMutationOptions(
  csrfToken: string,
  idempotencyKey: string,
): RequestInit | undefined {
  const options = mutationOptions(csrfToken);
  if (
    !options ||
    typeof idempotencyKey !== "string" ||
    idempotencyKey.length === 0 ||
    idempotencyKey.length > 200 ||
    idempotencyKey.trim() !== idempotencyKey
  ) {
    return undefined;
  }
  return {
    ...options,
    headers: { ...options.headers, "Idempotency-Key": idempotencyKey },
  };
}

function contactPolicyMutationFailure(
  status: number,
): Exclude<
  CustomerContactPolicyMutationResult,
  { readonly status: "succeeded" }
> {
  if (status === 401) return { status: "unauthenticated" };
  if (status === 403) return { status: "forbidden" };
  if (status === 404) return { status: "not_found" };
  if (status === 409) return { status: "conflict" };
  if (status === 400 || status === 422) return { status: "invalid" };
  return { status: "unavailable" };
}

export async function submitCustomerContactPolicySet(
  transport: CustomerDetailTransport,
  customerID: number,
  request: {
    readonly expectedVersion: number;
    readonly reasonCode: CustomerContactPolicyReason;
    readonly suppressedUntil: string | null;
  },
  csrfToken: string,
  idempotencyKey: string,
): Promise<CustomerContactPolicyMutationResult> {
  const options = contactPolicyMutationOptions(csrfToken, idempotencyKey);
  if (
    !positiveInteger(customerID) ||
    !options ||
    !nonNegativeInteger(request.expectedVersion) ||
    !contactPolicyReason(request.reasonCode) ||
    !(request.suppressedUntil === null || timestamp(request.suppressedUntil))
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.setContactPolicy(
      customerID,
      {
        expected_version: request.expectedVersion,
        reason_code: request.reasonCode,
        suppressed_until: request.suppressedUntil,
      },
      options,
    );
    if (response.status !== 200)
      return contactPolicyMutationFailure(response.status);
    const policy = parseCustomerContactPolicy(response.data, customerID);
    return policy
      ? { status: "succeeded", policy }
      : { status: "outcome_unknown" };
  } catch {
    return { status: "outcome_unknown" };
  }
}

export async function submitCustomerContactPolicyClear(
  transport: CustomerDetailTransport,
  customerID: number,
  expectedVersion: number,
  csrfToken: string,
  idempotencyKey: string,
): Promise<CustomerContactPolicyMutationResult> {
  const options = contactPolicyMutationOptions(csrfToken, idempotencyKey);
  if (
    !positiveInteger(customerID) ||
    !options ||
    !positiveInteger(expectedVersion)
  ) {
    return { status: "invalid" };
  }
  try {
    const response = await transport.clearContactPolicy(
      customerID,
      { expected_version: expectedVersion },
      options,
    );
    if (response.status !== 200)
      return contactPolicyMutationFailure(response.status);
    const policy = parseCustomerContactPolicy(response.data, customerID);
    return policy
      ? { status: "succeeded", policy }
      : { status: "outcome_unknown" };
  } catch {
    return { status: "outcome_unknown" };
  }
}

function validMutationCustomerID(customerID: number): boolean {
  return positiveInteger(customerID);
}

function validCustomerProfileUpdate(
  value: unknown,
): value is CustomerProfileUpdate {
  const keys = ["name", "avatarURL", "gender", "ownerStaffID", "channelID"];
  if (!exactObject(value, keys, keys)) return false;
  return (
    typeof value.name === "string" &&
    value.name.trim() !== "" &&
    (value.avatarURL === null || isSafeAvatarURL(value.avatarURL)) &&
    (value.gender === null || isCustomerGender(value.gender)) &&
    (value.ownerStaffID === null || positiveInteger(value.ownerStaffID)) &&
    (value.channelID === null || positiveInteger(value.channelID))
  );
}

function completeMutation(
  response: CustomerDetailTransportResponse,
  expectedStatus: number,
): CustomerMutationResult {
  return response.status === expectedStatus
    ? { status: "succeeded" }
    : { status: mutationFailure(response.status) };
}

export async function submitCustomerProfileUpdate(
  transport: CustomerDetailTransport,
  customerID: number,
  update: CustomerProfileUpdate,
  csrfToken: string,
): Promise<CustomerMutationResult> {
  const options = mutationOptions(csrfToken);
  if (
    !validMutationCustomerID(customerID) ||
    !options ||
    !validCustomerProfileUpdate(update)
  ) {
    return { status: "invalid" };
  }
  const request: CustomerUpdateRequest = {
    name: update.name,
    avatar_url: update.avatarURL,
    gender: update.gender,
    owner_staff_id: update.ownerStaffID,
    channel_id: update.channelID,
  };
  try {
    return completeMutation(
      await transport.update(customerID, request, options),
      200,
    );
  } catch {
    return { status: "unavailable" };
  }
}

export async function submitCustomerStageChange(
  transport: CustomerDetailTransport,
  customerID: number,
  stageID: number | null,
  csrfToken: string,
): Promise<CustomerMutationResult> {
  const options = mutationOptions(csrfToken);
  if (
    !validMutationCustomerID(customerID) ||
    !options ||
    (stageID !== null && !positiveInteger(stageID))
  ) {
    return { status: "invalid" };
  }
  try {
    return completeMutation(
      await transport.setStage(customerID, { stage_id: stageID }, options),
      200,
    );
  } catch {
    return { status: "unavailable" };
  }
}

async function submitTagMutation(
  operation: CustomerDetailTransport["addTag"],
  customerID: number,
  tagID: number,
  csrfToken: string,
): Promise<CustomerMutationResult> {
  const options = mutationOptions(csrfToken);
  if (
    !validMutationCustomerID(customerID) ||
    !positiveInteger(tagID) ||
    !options
  ) {
    return { status: "invalid" };
  }
  try {
    return completeMutation(await operation(customerID, tagID, options), 204);
  } catch {
    return { status: "unavailable" };
  }
}

export function submitCustomerTagAdd(
  transport: CustomerDetailTransport,
  customerID: number,
  tagID: number,
  csrfToken: string,
): Promise<CustomerMutationResult> {
  return submitTagMutation(transport.addTag, customerID, tagID, csrfToken);
}

export function submitCustomerTagRemoval(
  transport: CustomerDetailTransport,
  customerID: number,
  tagID: number,
  csrfToken: string,
): Promise<CustomerMutationResult> {
  return submitTagMutation(transport.removeTag, customerID, tagID, csrfToken);
}
