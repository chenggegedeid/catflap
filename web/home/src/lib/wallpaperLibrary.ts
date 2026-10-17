/**
 * 内置壁纸库。
 *
 * 和图标库同一个思路：让用户**点一下就有**，而不是必须上传或联网。
 * 全是纯 CSS 渐变 —— 不发请求、秒加载、4K 不糊，这是位图壁纸做不到的。
 *
 * 每套都是**多层径向渐变叠加**，比单条 linear-gradient 有层次；
 * 色相之间拉开了距离，方便用户一眼分辨自己用的是哪套。
 *
 * 存法：`wallpaper.mode = 'preset'`、`wallpaper.css = 内置 class 名`。
 * 用 class 而不是内联样式，是因为用户自定义 CSS 的覆盖机制
 * （`.default-wallpaper.default-wallpaper` 双类名提特异度）已经存在，沿用即可。
 */

export interface PresetWallpaper {
  key: string
  name: string
  /** 预览用的缩略背景（设置面板里显示） */
  preview: string
  /** 实际的 CSS background 值 */
  css: string
}

export const PRESET_WALLPAPERS: PresetWallpaper[] = [
  {
    key: 'aurora',
    name: '极光',
    preview: 'linear-gradient(135deg, #0f2027, #203a43, #2c5364)',
    css: 'radial-gradient(ellipse at 20% 20%, rgba(56,189,248,0.25), transparent 55%), radial-gradient(ellipse at 80% 70%, rgba(168,85,247,0.22), transparent 55%), linear-gradient(160deg, #0f2027, #203a43 55%, #2c5364)',
  },
  {
    key: 'sunset',
    name: '暮色',
    preview: 'linear-gradient(135deg, #2b1055, #7597de)',
    css: 'radial-gradient(ellipse at 75% 25%, rgba(251,146,60,0.30), transparent 55%), radial-gradient(ellipse at 25% 80%, rgba(139,92,246,0.28), transparent 55%), linear-gradient(160deg, #2b1055, #4b2064 55%, #7597de)',
  },
  {
    key: 'mint',
    name: '薄荷',
    preview: 'linear-gradient(135deg, #d1fae5, #6ee7b7, #34d399)',
    css: 'radial-gradient(ellipse at 25% 25%, rgba(255,255,255,0.5), transparent 55%), radial-gradient(ellipse at 80% 75%, rgba(16,185,129,0.25), transparent 55%), linear-gradient(160deg, #d1fae5, #6ee7b7 55%, #34d399)',
  },
  {
    key: 'dusk',
    name: '蓝调',
    preview: 'linear-gradient(135deg, #1e3a8a, #3b82f6, #93c5fd)',
    css: 'radial-gradient(ellipse at 70% 20%, rgba(147,197,253,0.35), transparent 55%), radial-gradient(ellipse at 20% 80%, rgba(30,58,138,0.5), transparent 55%), linear-gradient(160deg, #1e3a8a, #3b82f6 55%, #93c5fd)',
  },
  {
    key: 'ember',
    name: '余烬',
    preview: 'linear-gradient(135deg, #450a0a, #b91c1c, #f59e0b)',
    css: 'radial-gradient(ellipse at 80% 30%, rgba(245,158,11,0.28), transparent 55%), radial-gradient(ellipse at 20% 75%, rgba(185,28,28,0.35), transparent 55%), linear-gradient(160deg, #450a0a, #7f1d1d 55%, #b45309)',
  },
  {
    key: 'graphite',
    name: '石墨',
    preview: 'linear-gradient(135deg, #0f172a, #334155)',
    css: 'radial-gradient(ellipse at 30% 20%, rgba(148,163,184,0.18), transparent 55%), radial-gradient(ellipse at 75% 80%, rgba(51,65,85,0.5), transparent 55%), linear-gradient(160deg, #0f172a, #1e293b 55%, #334155)',
  },
]

/** 按 key 找预设；找不到返回 null */
export function findPreset(key: string): PresetWallpaper | null {
  return PRESET_WALLPAPERS.find((p) => p.key === key) ?? null
}
