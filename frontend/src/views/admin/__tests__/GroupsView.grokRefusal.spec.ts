import { defineComponent } from "vue";
import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AdminGroup } from "@/types";
import GroupsView from "@/views/admin/GroupsView.vue";

const { listGroups, getAccount, updateGroup, createGroup, getCandidates } = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getAccount: vi.fn(),
  updateGroup: vi.fn(),
  createGroup: vi.fn(),
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
      create: createGroup,
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
  createForm: {name: string; platform: string; grok_moderation_refusal_price: number | null};
  showCreateModal: boolean;
  closeCreateModal: () => void;
  handleCreateGroup: () => Promise<void>;
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


describe("Grok refusal group pricing", () => {
  beforeEach(() => {
    localStorage.clear(); vi.clearAllMocks();
    getCandidates.mockResolvedValue([]);
    getAccount.mockResolvedValue({id:101,name:"account"});
    listGroups.mockResolvedValue({items:[],total:0,pages:1});
    createGroup.mockResolvedValue(sourceGroup); updateGroup.mockResolvedValue(sourceGroup);
  });
  it.each(["grok", "composite"])("edits %s price, zero and null independently", async (platform) => {
    const wrapper=mountView(); await flushPromises(); const state=editState(wrapper);
    const group={...sourceGroup,platform,model_routing:null,grok_moderation_refusal_price:0.37} as AdminGroup;
    await state.handleEdit(group); await flushPromises();
    expect(wrapper.findAll('[data-testid="edit-grok-moderation-refusal-price"]')).toHaveLength(1);
    const input=wrapper.get('[data-testid="edit-grok-moderation-refusal-price"]');
    expect((input.element as HTMLInputElement).value).toBe("0.37");
    await input.setValue("0");await wrapper.get("#edit-group-form").trigger("submit");await flushPromises();
    expect(updateGroup).toHaveBeenLastCalledWith(42,expect.objectContaining({grok_moderation_refusal_price:0}));
    await state.handleEdit({...group,grok_moderation_refusal_price:0});await flushPromises();
    await wrapper.get('[data-testid="edit-grok-moderation-refusal-price"]').setValue("");
    await wrapper.get("#edit-group-form").trigger("submit");await flushPromises();
    expect(updateGroup).toHaveBeenLastCalledWith(42,expect.objectContaining({grok_moderation_refusal_price:null}));
    wrapper.unmount();
  });
  it("creates zero pricing and resets to the default", async () => {
    const wrapper=mountView();await flushPromises();const state=editState(wrapper);
    state.createForm.name="Grok";state.createForm.platform="grok";state.showCreateModal=true;await flushPromises();
    expect(wrapper.findAll('[data-testid="create-grok-moderation-refusal-price"]')).toHaveLength(1);
    await wrapper.get('[data-testid="create-grok-moderation-refusal-price"]').setValue("0");
    await state.handleCreateGroup();await flushPromises();
    expect(createGroup).toHaveBeenCalledWith(expect.objectContaining({grok_moderation_refusal_price:0}));
    state.closeCreateModal();expect(state.createForm.grok_moderation_refusal_price).toBeNull();wrapper.unmount();
  });
});
