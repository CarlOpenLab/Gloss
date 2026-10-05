import { ref } from 'vue'

/** 当前开着解释浮窗的 hunk id（同一时间只开一个，避免几个浮窗互相挡着） */
export const openExplainCard = ref<string | null>(null)
