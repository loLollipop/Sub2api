<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { OpenAIFastPolicyRule } from "@/api/admin/settings";
import Select from "@/components/common/Select.vue";
import OpenAIFastPolicyUserSelector from "./OpenAIFastPolicyUserSelector.vue";
const props = defineProps<{ policy: { rules: OpenAIFastPolicyRule[] } }>();
const { t } = useI18n();
// The container owns this object's identity, hydration, and guarded bulk save.
const openaiFastPolicyForm = props.policy;
// ==================== OpenAI Fast/Flex Policy ====================

const openaiFastPolicyTierOptions = computed(() => [
  { value: "all", label: t("admin.settings.openaiFastPolicy.tierAll") },
  {
    value: "priority",
    label: t("admin.settings.openaiFastPolicy.tierPriority"),
  },
  {
    value: "ultrafast",
    label: t("admin.settings.openaiFastPolicy.tierUltrafast"),
  },
  { value: "flex", label: t("admin.settings.openaiFastPolicy.tierFlex") },
  { value: "missing", label: t("admin.settings.openaiFastPolicy.tierMissing") },
]);

const openaiFastPolicyActionOptions = computed(() => [
  { value: "pass", label: t("admin.settings.openaiFastPolicy.actionPass") },
  { value: "filter", label: t("admin.settings.openaiFastPolicy.actionFilter") },
  {
    value: "force_priority",
    label: t("admin.settings.openaiFastPolicy.actionForcePriority"),
  },
  { value: "block", label: t("admin.settings.openaiFastPolicy.actionBlock") },
]);

function openaiFastPolicyActionSummary(
  action: OpenAIFastPolicyRule["action"],
) {
  return t(`admin.settings.openaiFastPolicy.summaryAction.${action}`);
}

function hasOpenAIFastPolicyTargetModels(rule: OpenAIFastPolicyRule) {
  return Boolean(rule.model_whitelist?.some((pattern) => pattern.trim() !== ""));
}

const openaiFastPolicyScopeOptions = computed(() => [
  { value: "all", label: t("admin.settings.openaiFastPolicy.scopeAll") },
  { value: "oauth", label: t("admin.settings.openaiFastPolicy.scopeOAuth") },
  { value: "apikey", label: t("admin.settings.openaiFastPolicy.scopeAPIKey") },
  {
    value: "bedrock",
    label: t("admin.settings.openaiFastPolicy.scopeBedrock"),
  },
]);

function addOpenAIFastPolicyRule() {
  openaiFastPolicyForm.rules.push({
    service_tier: "priority",
    action: "filter",
    scope: "all",
    user_ids: [],
    error_message: "",
    model_whitelist: [],
    fallback_action: "pass",
    fallback_error_message: "",
  });
}

function removeOpenAIFastPolicyRule(index: number) {
  openaiFastPolicyForm.rules.splice(index, 1);
}

function addOpenAIFastPolicyModelPattern(rule: OpenAIFastPolicyRule) {
  if (!rule.model_whitelist) rule.model_whitelist = [];
  rule.model_whitelist.push("");
}

function removeOpenAIFastPolicyModelPattern(
  rule: OpenAIFastPolicyRule,
  idx: number,
) {
  rule.model_whitelist?.splice(idx, 1);
}


</script>

<template>
<div class="card">
  <div
    class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
  >
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
      {{ t("admin.settings.openaiFastPolicy.title") }}
    </h2>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.settings.openaiFastPolicy.description") }}
    </p>
  </div>
  <div class="space-y-5 p-6">
    <!-- Empty state -->
    <div
      v-if="openaiFastPolicyForm.rules.length === 0"
      class="rounded-lg border border-dashed border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
    >
      {{ t("admin.settings.openaiFastPolicy.empty") }}
    </div>

    <!-- Rule Cards -->
    <div
      v-for="(rule, ruleIndex) in openaiFastPolicyForm.rules"
      :key="ruleIndex"
      class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
    >
      <div class="mb-3 flex items-center justify-between">
        <span
          class="text-sm font-medium text-gray-900 dark:text-white"
        >
          {{
            t("admin.settings.openaiFastPolicy.ruleHeader", {
              index: ruleIndex + 1,
            })
          }}
        </span>
        <button
          type="button"
          @click="removeOpenAIFastPolicyRule(ruleIndex)"
          class="rounded p-1 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
          :title="t('admin.settings.openaiFastPolicy.removeRule')"
        >
          <Icon name="x" size="none" class="h-4 w-4" :stroke-width="2" />
        </button>
      </div>

      <div
        class="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500 dark:text-gray-400"
        :data-testid="`openai-fast-policy-summary-${ruleIndex}`"
      >
        <span class="font-medium text-gray-700 dark:text-gray-300">
          {{
            t(
              hasOpenAIFastPolicyTargetModels(rule)
                ? "admin.settings.openaiFastPolicy.summaryTargetModels"
                : "admin.settings.openaiFastPolicy.summaryAllModels",
            )
          }}
        </span>
        <span aria-hidden="true">→</span>
        <span
          class="inline-flex items-center rounded bg-primary-50 px-2 py-0.5 font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
        >
          {{ openaiFastPolicyActionSummary(rule.action) }}
        </span>
        <template v-if="hasOpenAIFastPolicyTargetModels(rule)">
          <span aria-hidden="true">·</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">
            {{
              t(
                "admin.settings.openaiFastPolicy.summaryOtherModels",
              )
            }}
          </span>
          <span aria-hidden="true">→</span>
          <span
            class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 font-medium text-gray-700 dark:bg-dark-600 dark:text-gray-300"
          >
            {{
              openaiFastPolicyActionSummary(
                rule.fallback_action || "pass",
              )
            }}
          </span>
        </template>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
        <!-- Service Tier -->
        <div>
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.serviceTier") }}
          </label>
          <Select
            :modelValue="rule.service_tier"
            @update:modelValue="
              rule.service_tier = $event as
                | 'all'
                | 'priority'
                | 'flex'
                | 'missing'
            "
            :options="openaiFastPolicyTierOptions"
          />
        </div>

        <!-- Action -->
        <div>
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.action") }}
          </label>
          <Select
            :modelValue="rule.action"
            @update:modelValue="
              rule.action = $event as
                | 'pass'
                | 'filter'
                | 'block'
                | 'force_priority'
            "
            :options="openaiFastPolicyActionOptions"
          />
        </div>

        <!-- Scope -->
        <div>
          <label
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.openaiFastPolicy.scope") }}
          </label>
          <Select
            :modelValue="rule.scope"
            @update:modelValue="
              rule.scope = $event as
                | 'all'
                | 'oauth'
                | 'apikey'
                | 'bedrock'
            "
            :options="openaiFastPolicyScopeOptions"
          />
        </div>
      </div>

      <!-- User Scope -->
      <div class="mt-3">
        <label
          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
        >
          {{ t("admin.settings.openaiFastPolicy.userIds") }}
        </label>
        <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
          {{ t("admin.settings.openaiFastPolicy.userIdsHint") }}
        </p>
        <OpenAIFastPolicyUserSelector
          :model-value="rule.user_ids || []"
          @update:model-value="rule.user_ids = $event"
        />
      </div>

      <!-- Error Message (only when action=block) -->
      <div v-if="rule.action === 'block'" class="mt-3">
        <label
          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
        >
          {{ t("admin.settings.openaiFastPolicy.errorMessage") }}
        </label>
        <input
          v-model="rule.error_message"
          type="text"
          class="input"
          :placeholder="
            t(
              'admin.settings.openaiFastPolicy.errorMessagePlaceholder',
            )
          "
        />
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
          {{ t("admin.settings.openaiFastPolicy.errorMessageHint") }}
        </p>
      </div>

      <!-- Target Models -->
      <div
        class="mt-3"
        role="group"
        :aria-labelledby="`openai-fast-policy-models-label-${ruleIndex}`"
        :aria-describedby="`openai-fast-policy-models-hint-${ruleIndex}`"
      >
        <label
          :id="`openai-fast-policy-models-label-${ruleIndex}`"
          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
        >
          {{ t("admin.settings.openaiFastPolicy.modelWhitelist") }}
        </label>
        <p
          :id="`openai-fast-policy-models-hint-${ruleIndex}`"
          class="mb-2 text-xs text-gray-400 dark:text-gray-500"
        >
          {{
            t("admin.settings.openaiFastPolicy.modelWhitelistHint")
          }}
        </p>
        <div
          v-for="(_, patternIdx) in rule.model_whitelist || []"
          :key="patternIdx"
          class="mb-1.5 flex items-center gap-2"
        >
          <input
            v-model="rule.model_whitelist![patternIdx]"
            type="text"
            class="input input-sm flex-1"
            :placeholder="
              t(
                'admin.settings.openaiFastPolicy.modelPatternPlaceholder',
              )
            "
          />
          <button
            type="button"
            @click="
              removeOpenAIFastPolicyModelPattern(rule, patternIdx)
            "
            class="shrink-0 rounded p-1 text-red-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
          >
            <Icon name="x" size="none" class="h-4 w-4" :stroke-width="2" />
          </button>
        </div>
        <button
          type="button"
          @click="addOpenAIFastPolicyModelPattern(rule)"
          class="mb-2 inline-flex items-center gap-1 text-xs text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
        >
          <Icon name="plusWide" size="none" class="h-3.5 w-3.5" :stroke-width="2" />
          {{ t("admin.settings.openaiFastPolicy.addModelPattern") }}
        </button>
      </div>

      <!-- Other Models Action (only when target models are non-empty) -->
      <div
        v-if="hasOpenAIFastPolicyTargetModels(rule)"
        class="mt-3"
      >
        <label
          class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
        >
          {{ t("admin.settings.openaiFastPolicy.fallbackAction") }}
        </label>
        <Select
          :modelValue="rule.fallback_action || 'pass'"
          @update:modelValue="
            rule.fallback_action = $event as
              | 'pass'
              | 'filter'
              | 'block'
              | 'force_priority'
          "
          :options="openaiFastPolicyActionOptions"
        />
        <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">
          {{
            t("admin.settings.openaiFastPolicy.fallbackActionHint")
          }}
        </p>
        <div v-if="rule.fallback_action === 'block'" class="mt-2">
          <input
            v-model="rule.fallback_error_message"
            type="text"
            class="input"
            :placeholder="
              t(
                'admin.settings.openaiFastPolicy.fallbackErrorMessagePlaceholder',
              )
            "
          />
        </div>
      </div>
    </div>

    <!-- Add Rule Button -->
    <div>
      <button
        type="button"
        @click="addOpenAIFastPolicyRule"
        class="btn btn-secondary btn-sm inline-flex items-center gap-1"
      >
        <Icon name="plusWide" size="none" class="h-4 w-4" :stroke-width="2" />
        {{ t("admin.settings.openaiFastPolicy.addRule") }}
      </button>
      <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
        {{ t("admin.settings.openaiFastPolicy.saveHint") }}
      </p>
    </div>
  </div>
</div>
</template>
