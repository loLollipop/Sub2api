import { defineComponent } from "vue";
import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AdminGroup } from "@/types";
import GroupsView from "@/views/admin/GroupsView.vue";

const { listGroups, getAccount, updateGroup, getCandidates } = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getAccount: vi.fn(),
  updateGroup: vi.fn(),
  getCandidates: vi.fn(),
}));

vi.mock("@/api/admin", () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getAll: vi.fn(),
      getModelAllowlistCandidates: getCandidates,
      getUsageSummary: vi.fn().mockResolvedValue([]),
      getCapacitySummary: vi.fn().mockResolvedValue([]),
      getLiveCapability: vi.fn().mockResolvedValue({ supported: false }),
      update: updateGroup,
    },
    accounts: { getById: getAccount, list: vi.fn() },
  },
}));
vi.mock("@/stores/app", () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}));
vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({ isSimpleMode: false }),
}));
vi.mock("@/stores/onboarding", () => ({
  useOnboardingStore: () => ({ isCurrentStep: () => false }),
}));
vi.mock("vue-i18n", async () => ({
  ...await vi.importActual<typeof import("vue-i18n")>("vue-i18n"),
  useI18n: () => ({ t: (key: string) => key }),
}));

const sourceGroup: AdminGroup = {
  id: 42,
  name: "Group A",
  description: null,
  platform: "openai",
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: "active",
  subscription_type: "standard",
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 0.5,
  batch_image_hold_multiplier: 0.6,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  peak_rate_enabled: false,
  peak_start: "",
  peak_end: "",
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: "2026-10-15T00:00:00Z",
  updated_at: "2026-10-15T00:00:00Z",
  model_routing: { "model-a": [101] },
  model_routing_enabled: true,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  account_count: 1,
  active_account_count: 1,
  rate_limited_account_count: 0,
  sort_order: 10,
};
const groupB = {
  ...sourceGroup,
  id: 43,
  name: "Group B",
  model_routing: { "model-b": [102] },
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((complete) => { resolve = complete; });
  return { promise, resolve };
}

const SlotStub = defineComponent({ template: "<div><slot /></div>" });
const TableStub = defineComponent({
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
});
const DialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
});
const ValidatedFieldStub = defineComponent({
  setup(_, { expose }) {
    expose({ validate: () => true, resetValidation: () => undefined });
    return () => null;
  },
});
function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: SlotStub,
        TablePageLayout: TableStub,
        BaseDialog: DialogStub,
        DataTable: true,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        PlatformIcon: true,
        Icon: true,
        GroupI18nField: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        ReasoningEffortPolicyFields: ValidatedFieldStub,
        CodexManifestAccountsField: ValidatedFieldStub,
        PricingEntryCard: true,
        VueDraggable: true,
      },
    },
  });
}

interface EditState {
  handleEdit: (group: AdminGroup) => Promise<void>;
  closeEditModal: () => void;
  handleUpdateGroup: () => Promise<void>;
  showEditModal: boolean;
  editingGroup: AdminGroup | null;
  editModelRoutingRules: { pattern: string; accounts: { id: number; name: string }[] }[];
  editCodexManifestAccountNames: Record<number, string>;
  editModelAllowlistState: { items: unknown[] };
  editModelAllowlistLoading: boolean;
}
function editState(wrapper: ReturnType<typeof mountView>) {
  return (wrapper.vm as unknown as { $: { setupState: EditState } }).$.setupState;
}

describe("GroupsView edit request ownership", () => {
  const wrappers: ReturnType<typeof mountView>[] = [];
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    getAccount.mockReset();
    getCandidates.mockReset().mockResolvedValue([]);
    listGroups.mockResolvedValue({ items: [sourceGroup, groupB], total: 2, pages: 1 });
    updateGroup.mockResolvedValue(sourceGroup);
  });
  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount());
  });
  async function openView() {
    const wrapper = mountView();
    wrappers.push(wrapper);
    await flushPromises();
    return { wrapper, state: editState(wrapper) };
  }

  it("keeps B's routes and save target when A finishes after B", async () => {
    const a = deferred<{ id: number; name: string }>();
    const b = deferred<{ id: number; name: string }>();
    getAccount.mockImplementation((id: number) => id === 101 ? a.promise : b.promise);
    const { wrapper, state } = await openView();
    const editA = state.handleEdit(sourceGroup);
    const editB = state.handleEdit(groupB);
    b.resolve({ id: 102, name: "Account B" });
    await editB;
    a.resolve({ id: 101, name: "Account A" });
    await editA;
    await flushPromises();

    expect(state.editingGroup?.id).toBe(43);
    expect(state.editModelRoutingRules).toEqual([
      { pattern: "model-b", accounts: [{ id: 102, name: "Account B" }] },
    ]);
    await wrapper.get("#edit-group-form").trigger("submit");
    await flushPromises();
    expect(updateGroup).toHaveBeenCalledWith(43, expect.objectContaining({
      model_routing: { "model-b": [102] },
    }));
  });

  it("clears the old draft and prevents saving while the next group's routes load", async () => {
    getAccount.mockResolvedValueOnce({ id: 101, name: "Account A" });
    const b = deferred<{ id: number; name: string }>();
    getAccount.mockReturnValueOnce(b.promise);
    const { wrapper, state } = await openView();
    await state.handleEdit(sourceGroup);
    await flushPromises();
    expect(wrapper.find("#edit-group-form").exists()).toBe(true);

    const editB = state.handleEdit(groupB);
    await flushPromises();
    expect(state.showEditModal).toBe(false);
    expect(state.editModelRoutingRules).toEqual([]);
    expect(wrapper.find("#edit-group-form").exists()).toBe(false);
    await state.handleUpdateGroup();
    expect(updateGroup).not.toHaveBeenCalled();
    b.resolve({ id: 102, name: "Account B" });
    await editB;
    expect(state.showEditModal).toBe(true);
  });

  it("does not reopen a closed dialog when routing account resolution finishes", async () => {
    const account = deferred<{ id: number; name: string }>();
    getAccount.mockReturnValue(account.promise);
    const { state } = await openView();
    const edit = state.handleEdit(sourceGroup);
    state.closeEditModal();
    account.resolve({ id: 101, name: "Account A" });
    await edit;
    expect(state.showEditModal).toBe(false);
    expect(state.editingGroup).toBeNull();
    expect(state.editModelRoutingRules).toEqual([]);
  });

  it("ignores old routing and manifest responses after closing and reopening the same group", async () => {
    const oldManifest = deferred<{ id: number; name: string }>();
    const oldRoute = deferred<{ id: number; name: string }>();
    getAccount
      .mockReturnValueOnce(oldManifest.promise)
      .mockReturnValueOnce(oldRoute.promise)
      .mockResolvedValueOnce({ id: 17, name: "Current manifest" })
      .mockResolvedValueOnce({ id: 101, name: "Current route" });
    const group = {
      ...sourceGroup,
      codex_models_manifest_config: { enabled: true, account_ids: [17], fallback_to_scheduler: false },
    };
    const { state } = await openView();
    const oldEdit = state.handleEdit(group);
    state.closeEditModal();
    await state.handleEdit(group);
    oldManifest.resolve({ id: 17, name: "Old manifest" });
    oldRoute.resolve({ id: 101, name: "Old route" });
    await oldEdit;
    await flushPromises();
    expect(state.editCodexManifestAccountNames).toEqual({ 17: "Current manifest" });
    expect(state.editModelRoutingRules[0].accounts[0].name).toBe("Current route");
    expect(state.showEditModal).toBe(true);
  });

  it("ignores routing and manifest responses after unmount", async () => {
    const manifest = deferred<{ id: number; name: string }>();
    const route = deferred<{ id: number; name: string }>();
    getAccount.mockReturnValueOnce(manifest.promise).mockReturnValueOnce(route.promise);
    const { wrapper, state } = await openView();
    const edit = state.handleEdit({
      ...sourceGroup,
      codex_models_manifest_config: { enabled: true, account_ids: [17], fallback_to_scheduler: false },
    });
    await flushPromises();
    const candidateCalls = getCandidates.mock.calls.length;
    wrapper.unmount();
    manifest.resolve({ id: 17, name: "Late manifest" });
    route.resolve({ id: 101, name: "Late route" });
    await edit;
    await flushPromises();
    expect(state.editCodexManifestAccountNames).toEqual({});
    expect(state.editModelRoutingRules).toEqual([]);
    expect(state.showEditModal).toBe(false);
    expect(getCandidates).toHaveBeenCalledTimes(candidateCalls);
  });

  it.each(["close", "unmount"] as const)("ignores pending allowlist candidates after %s", async (action) => {
    const { wrapper, state } = await openView();
    const candidates = deferred<string[]>();
    getCandidates.mockReturnValue(candidates.promise);
    await state.handleEdit({ ...sourceGroup, model_routing: null });
    await flushPromises();
    expect(state.editModelAllowlistLoading).toBe(true);
    if (action === "close") state.closeEditModal();
    else wrapper.unmount();
    const previousItems = [...state.editModelAllowlistState.items];
    candidates.resolve(["late-model"]);
    await flushPromises();
    expect(state.editModelAllowlistState.items).toEqual(previousItems);
    if (action === "close") expect(state.editModelAllowlistLoading).toBe(false);
  });
});
