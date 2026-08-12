import {
  addCustomerTag,
  getCustomer,
  listCustomerEvents,
  listTags,
  removeCustomerTag,
  setCustomerStage,
  updateCustomer,
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

export interface CustomerDetailSnapshot {
  readonly customer: CustomerProfile;
  readonly tags: readonly CustomerTag[];
  readonly tagCatalog: readonly CustomerTag[];
  readonly events: readonly CustomerTimelineEvent[];
  readonly eventsHaveMore: boolean;
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

export interface CustomerDetailTransport {
  readonly get: typeof loadGeneratedCustomer;
  readonly update: typeof updateGeneratedCustomer;
  readonly setStage: typeof setGeneratedCustomerStage;
  readonly addTag: typeof addGeneratedCustomerTag;
  readonly removeTag: typeof removeGeneratedCustomerTag;
  readonly listEvents: typeof loadGeneratedCustomerEvents;
  readonly listTags: typeof loadGeneratedTags;
}

export const generatedCustomerDetailTransport: CustomerDetailTransport = {
  get: loadGeneratedCustomer,
  update: updateGeneratedCustomer,
  setStage: setGeneratedCustomerStage,
  addTag: addGeneratedCustomerTag,
  removeTag: removeGeneratedCustomerTag,
  listEvents: loadGeneratedCustomerEvents,
  listTags: loadGeneratedTags,
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

export type CustomerMutationFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "invalid"
  | "unavailable";

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
      readonly haveMore: boolean;
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
  return { events, haveMore: typeof value.next_cursor === "string" };
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
        eventsHaveMore: parsedEvents.haveMore,
      },
    };
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
