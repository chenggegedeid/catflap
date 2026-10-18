/**
 * 所有弹窗共用的遮罩样式。
 *
 * `grid-cols-1` 不能省。不写的话隐式列是 `auto`，宽度按里面内容的 max-content 算，
 * 弹窗内容只要有一层固定上限（比如 Webhook 那页的 `max-w-2xl`），
 * 这一列就被撑到 720px 并且不再跟着屏幕缩——手机上弹窗比屏幕还宽，
 * 右半边的输入框直接看不见。`grid-cols-1` 展开是 `minmax(0, 1fr)`，
 * 把列钉在容器宽度上，弹窗才会跟着屏幕走。
 *
 * `catflap-modal` 这个钩子是给 CSS 用的（见 index.css）：手机上把弹窗改成
 * **贴底抽屉**。放在 CSS 而不是这里写 `sm:` 变体，是因为弹窗有几十个，
 * 抽屉化要同时改遮罩定位和弹窗本体（圆角、宽度），逐个改容易漏；
 * 用一条 CSS 选择器命中 `.catflap-modal > *` 就一次生效，以后新弹窗自动继承。
 */
export const modalBackdrop =
  'catflap-modal fixed inset-0 z-50 grid grid-cols-1 place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm'
