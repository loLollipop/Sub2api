<script lang="ts">
import { ref, reactive, toRefs } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";
import Toggle from "@/components/common/Toggle.vue";

// Created by SettingsView so requests start in the original eager load sequence,
// even while the bulk settings form is still hidden by its loading state.
export function useOverloadCooldownCard() {
  const { t } = useI18n();
  const appStore = useAppStore();
  // Overload Cooldown (529) 状态
  const overloadCooldownLoading = ref(true);
  const overloadCooldownSaving = ref(false);
  const overloadCooldownForm = reactive({
    enabled: true,
    cooldown_minutes: 10,
  });

  // Overload Cooldown 方法
  async function loadOverloadCooldownSettings() {
    overloadCooldownLoading.value = true;
    try {
      const settings = await adminAPI.settings.getOverloadCooldownSettings();
      Object.assign(overloadCooldownForm, settings);
    } catch (_error: unknown) {
      // Silent fail - settings will use defaults
    } finally {
      overloadCooldownLoading.value = false;
    }
  }

  async function saveOverloadCooldownSettings() {
    overloadCooldownSaving.value = true;
    try {
      const updated = await adminAPI.settings.updateOverloadCooldownSettings({
        enabled: overloadCooldownForm.enabled,
        cooldown_minutes: overloadCooldownForm.cooldown_minutes,
      });
      Object.assign(overloadCooldownForm, updated);
      appStore.showSuccess(t("admin.settings.overloadCooldown.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.overloadCooldown.saveFailed"),
        ),
      );
    } finally {
      overloadCooldownSaving.value = false;
    }
  }
  return reactive({ overloadCooldownLoading, overloadCooldownSaving, overloadCooldownForm, loadOverloadCooldownSettings, saveOverloadCooldownSettings });
}
</script>

<script setup lang="ts">
const props = defineProps<{ controller: ReturnType<typeof useOverloadCooldownCard> }>();
const { t } = useI18n();
const { overloadCooldownLoading, overloadCooldownSaving, overloadCooldownForm, saveOverloadCooldownSettings } = toRefs(props.controller);
</script>

<template>
<div class="card">
  <div
    class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
  >
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
      {{ t("admin.settings.overloadCooldown.title") }}
    </h2>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.settings.overloadCooldown.description") }}
    </p>
  </div>
  <div class="space-y-5 p-6">
    <div
      v-if="overloadCooldownLoading"
      class="flex items-center gap-2 text-gray-500"
    >
      <div
        class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
      ></div>
      {{ t("common.loading") }}
    </div>

    <template v-else>
      <div class="flex items-center justify-between">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.overloadCooldown.enabled")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.overloadCooldown.enabledHint") }}
          </p>
        </div>
        <Toggle v-model="overloadCooldownForm.enabled" />
      </div>

      <div
        v-if="overloadCooldownForm.enabled"
        class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.overloadCooldown.cooldownMinutes") }}
          </label>
          <input
            v-model.number="overloadCooldownForm.cooldown_minutes"
            type="number"
            min="1"
            max="120"
            class="input w-32"
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.overloadCooldown.cooldownMinutesHint")
            }}
          </p>
        </div>
      </div>

      <div
        class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <button
          type="button"
          @click="saveOverloadCooldownSettings"
          :disabled="overloadCooldownSaving"
          class="btn btn-primary btn-sm"
        >
          <svg
            v-if="overloadCooldownSaving"
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
            overloadCooldownSaving
              ? t("common.saving")
              : t("common.save")
          }}
        </button>
      </div>
    </template>
  </div>
</div>
</template>
