<script lang="ts">
import { ref, reactive, toRefs } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";
import Toggle from "@/components/common/Toggle.vue";

// Created by SettingsView so requests start in the original eager load sequence,
// even while the bulk settings form is still hidden by its loading state.
export function useRequestRectifierCard() {
  const { t } = useI18n();
  const appStore = useAppStore();
  // Rectifier 状态
  const rectifierLoading = ref(true);
  const rectifierSaving = ref(false);
  const rectifierForm = reactive({
    enabled: true,
    thinking_signature_enabled: true,
    thinking_budget_enabled: true,
    apikey_signature_enabled: false,
    apikey_signature_patterns: [] as string[],
  });

  // Rectifier 方法
  async function loadRectifierSettings() {
    rectifierLoading.value = true;
    try {
      const settings = await adminAPI.settings.getRectifierSettings();
      Object.assign(rectifierForm, settings);
      // 确保 patterns 是数组（旧数据可能为 null）
      if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
        rectifierForm.apikey_signature_patterns = [];
      }
    } catch (_error: unknown) {
      // Silent fail - settings will use defaults
    } finally {
      rectifierLoading.value = false;
    }
  }

  async function saveRectifierSettings() {
    rectifierSaving.value = true;
    try {
      const updated = await adminAPI.settings.updateRectifierSettings({
        enabled: rectifierForm.enabled,
        thinking_signature_enabled: rectifierForm.thinking_signature_enabled,
        thinking_budget_enabled: rectifierForm.thinking_budget_enabled,
        apikey_signature_enabled: rectifierForm.apikey_signature_enabled,
        apikey_signature_patterns: rectifierForm.apikey_signature_patterns.filter(
          (p) => p.trim() !== "",
        ),
      });
      Object.assign(rectifierForm, updated);
      if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
        rectifierForm.apikey_signature_patterns = [];
      }
      appStore.showSuccess(t("admin.settings.rectifier.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(error, t("admin.settings.rectifier.saveFailed")),
      );
    } finally {
      rectifierSaving.value = false;
    }
  }
  return reactive({ rectifierLoading, rectifierSaving, rectifierForm, loadRectifierSettings, saveRectifierSettings });
}
</script>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
const props = defineProps<{ controller: ReturnType<typeof useRequestRectifierCard> }>();
const { t } = useI18n();
const { rectifierLoading, rectifierSaving, rectifierForm, saveRectifierSettings } = toRefs(props.controller);
</script>

<template>
<div class="card">
  <div
    class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
  >
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
      {{ t("admin.settings.rectifier.title") }}
    </h2>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.settings.rectifier.description") }}
    </p>
  </div>
  <div class="space-y-5 p-6">
    <!-- Loading State -->
    <div
      v-if="rectifierLoading"
      class="flex items-center gap-2 text-gray-500"
    >
      <div
        class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
      ></div>
      {{ t("common.loading") }}
    </div>

    <template v-else>
      <!-- Master Toggle -->
      <div class="flex items-center justify-between">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.rectifier.enabled")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.rectifier.enabledHint") }}
          </p>
        </div>
        <Toggle v-model="rectifierForm.enabled" />
      </div>

      <!-- Sub-toggles (only show when master is enabled) -->
      <div
        v-if="rectifierForm.enabled"
        class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <!-- Thinking Signature Rectifier -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >{{
                t("admin.settings.rectifier.thinkingSignature")
              }}</label
            >
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{
                t("admin.settings.rectifier.thinkingSignatureHint")
              }}
            </p>
          </div>
          <Toggle
            v-model="rectifierForm.thinking_signature_enabled"
          />
        </div>

        <!-- Thinking Budget Rectifier -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >{{
                t("admin.settings.rectifier.thinkingBudget")
              }}</label
            >
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.rectifier.thinkingBudgetHint") }}
            </p>
          </div>
          <Toggle v-model="rectifierForm.thinking_budget_enabled" />
        </div>

        <!-- API Key Signature Rectifier -->
        <div class="flex items-center justify-between">
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >{{
                t("admin.settings.rectifier.apikeySignature")
              }}</label
            >
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.rectifier.apikeySignatureHint") }}
            </p>
          </div>
          <Toggle v-model="rectifierForm.apikey_signature_enabled" />
        </div>

        <!-- Custom Patterns (only when apikey_signature_enabled) -->
        <div
          v-if="rectifierForm.apikey_signature_enabled"
          class="ml-4 space-y-3 border-l-2 border-gray-200 pl-4 dark:border-dark-600"
        >
          <div>
            <label
              class="text-sm font-medium text-gray-700 dark:text-gray-300"
              >{{
                t("admin.settings.rectifier.apikeyPatterns")
              }}</label
            >
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.rectifier.apikeyPatternsHint") }}
            </p>
          </div>
          <div
            v-for="(
              _, index
            ) in rectifierForm.apikey_signature_patterns"
            :key="index"
            class="flex items-center gap-2"
          >
            <input
              v-model="rectifierForm.apikey_signature_patterns[index]"
              type="text"
              class="input input-sm flex-1"
              :placeholder="
                t('admin.settings.rectifier.apikeyPatternPlaceholder')
              "
            />
            <button
              type="button"
              @click="
                rectifierForm.apikey_signature_patterns.splice(
                  index,
                  1,
                )
              "
              class="btn btn-ghost btn-xs text-red-500 hover:text-red-700"
            >
              <Icon name="x" size="none" class="h-4 w-4" :stroke-width="2" />
            </button>
          </div>
          <button
            type="button"
            @click="rectifierForm.apikey_signature_patterns.push('')"
            class="btn btn-ghost btn-xs text-primary-600 dark:text-primary-400"
          >
            + {{ t("admin.settings.rectifier.addPattern") }}
          </button>
        </div>
      </div>

      <!-- Save Button -->
      <div
        class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <button
          type="button"
          @click="saveRectifierSettings"
          :disabled="rectifierSaving"
          class="btn btn-primary btn-sm"
        >
          <svg
            v-if="rectifierSaving"
            class="mr-1 h-4 w-4 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{
            rectifierSaving ? t("common.saving") : t("common.save")
          }}
        </button>
      </div>
    </template>
  </div>
</div>
</template>
