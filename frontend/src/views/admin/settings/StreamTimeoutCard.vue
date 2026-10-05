<script lang="ts">
import { ref, reactive, toRefs } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";
import Toggle from "@/components/common/Toggle.vue";

// Created by SettingsView so requests start in the original eager load sequence,
// even while the bulk settings form is still hidden by its loading state.
export function useStreamTimeoutCard() {
  const { t } = useI18n();
  const appStore = useAppStore();
  // Stream Timeout 状态
  const streamTimeoutLoading = ref(true);
  const streamTimeoutSaving = ref(false);
  const streamTimeoutForm = reactive({
    enabled: true,
    action: "temp_unsched" as "temp_unsched" | "error" | "none",
    temp_unsched_minutes: 5,
    threshold_count: 3,
    threshold_window_minutes: 10,
  });

  // Stream Timeout 方法
  async function loadStreamTimeoutSettings() {
    streamTimeoutLoading.value = true;
    try {
      const settings = await adminAPI.settings.getStreamTimeoutSettings();
      Object.assign(streamTimeoutForm, settings);
    } catch (_error: unknown) {
      // Silent fail - settings will use defaults
    } finally {
      streamTimeoutLoading.value = false;
    }
  }

  async function saveStreamTimeoutSettings() {
    streamTimeoutSaving.value = true;
    try {
      const updated = await adminAPI.settings.updateStreamTimeoutSettings({
        enabled: streamTimeoutForm.enabled,
        action: streamTimeoutForm.action,
        temp_unsched_minutes: streamTimeoutForm.temp_unsched_minutes,
        threshold_count: streamTimeoutForm.threshold_count,
        threshold_window_minutes: streamTimeoutForm.threshold_window_minutes,
      });
      Object.assign(streamTimeoutForm, updated);
      appStore.showSuccess(t("admin.settings.streamTimeout.saved"));
    } catch (error: unknown) {
      appStore.showError(
        extractApiErrorMessage(
          error,
          t("admin.settings.streamTimeout.saveFailed"),
        ),
      );
    } finally {
      streamTimeoutSaving.value = false;
    }
  }
  return reactive({ streamTimeoutLoading, streamTimeoutSaving, streamTimeoutForm, loadStreamTimeoutSettings, saveStreamTimeoutSettings });
}
</script>

<script setup lang="ts">
const props = defineProps<{ controller: ReturnType<typeof useStreamTimeoutCard> }>();
const { t } = useI18n();
const { streamTimeoutLoading, streamTimeoutSaving, streamTimeoutForm, saveStreamTimeoutSettings } = toRefs(props.controller);
</script>

<template>
<div class="card">
  <div
    class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
  >
    <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
      {{ t("admin.settings.streamTimeout.title") }}
    </h2>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      {{ t("admin.settings.streamTimeout.description") }}
    </p>
  </div>
  <div class="space-y-5 p-6">
    <!-- Loading State -->
    <div
      v-if="streamTimeoutLoading"
      class="flex items-center gap-2 text-gray-500"
    >
      <div
        class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600"
      ></div>
      {{ t("common.loading") }}
    </div>

    <template v-else>
      <!-- Enable Stream Timeout -->
      <div class="flex items-center justify-between">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{
            t("admin.settings.streamTimeout.enabled")
          }}</label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.streamTimeout.enabledHint") }}
          </p>
        </div>
        <Toggle v-model="streamTimeoutForm.enabled" />
      </div>

      <!-- Settings - Only show when enabled -->
      <div
        v-if="streamTimeoutForm.enabled"
        class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <!-- Action -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.streamTimeout.action") }}
          </label>
          <select
            v-model="streamTimeoutForm.action"
            class="input w-64"
          >
            <option value="temp_unsched">
              {{
                t("admin.settings.streamTimeout.actionTempUnsched")
              }}
            </option>
            <option value="error">
              {{ t("admin.settings.streamTimeout.actionError") }}
            </option>
            <option value="none">
              {{ t("admin.settings.streamTimeout.actionNone") }}
            </option>
          </select>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.streamTimeout.actionHint") }}
          </p>
        </div>

        <!-- Temp Unsched Minutes (only show when action is temp_unsched) -->
        <div v-if="streamTimeoutForm.action === 'temp_unsched'">
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.streamTimeout.tempUnschedMinutes") }}
          </label>
          <input
            v-model.number="streamTimeoutForm.temp_unsched_minutes"
            type="number"
            min="1"
            max="60"
            class="input w-32"
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t("admin.settings.streamTimeout.tempUnschedMinutesHint")
            }}
          </p>
        </div>

        <!-- Threshold Count -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.settings.streamTimeout.thresholdCount") }}
          </label>
          <input
            v-model.number="streamTimeoutForm.threshold_count"
            type="number"
            min="1"
            max="10"
            class="input w-32"
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.streamTimeout.thresholdCountHint") }}
          </p>
        </div>

        <!-- Threshold Window Minutes -->
        <div>
          <label
            class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{
              t("admin.settings.streamTimeout.thresholdWindowMinutes")
            }}
          </label>
          <input
            v-model.number="
              streamTimeoutForm.threshold_window_minutes
            "
            type="number"
            min="1"
            max="60"
            class="input w-32"
          />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{
              t(
                "admin.settings.streamTimeout.thresholdWindowMinutesHint",
              )
            }}
          </p>
        </div>
      </div>

      <!-- Save Button -->
      <div
        class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <button
          type="button"
          @click="saveStreamTimeoutSettings"
          :disabled="streamTimeoutSaving"
          class="btn btn-primary btn-sm"
        >
          <svg
            v-if="streamTimeoutSaving"
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
            streamTimeoutSaving
              ? t("common.saving")
              : t("common.save")
          }}
        </button>
      </div>
    </template>
  </div>
</div>
</template>
