import { Component, type ReactNode } from 'react'

/**
 * 页面级错误边界。
 *
 * **为什么需要**：任何一个页面渲染时抛异常，如果没有边界兜底，
 * React 会把整棵树卸载 —— 表现是**全站白屏**（连侧栏都没了），
 * 控制台里只有一句压缩过的报错，用户说不清、我们也难定位。
 *
 * 套上之后：崩的只是那一页，其它页面照常。
 *
 * **错误详情要显示出来**：生产构建里 React 的错误被压缩成
 * 「Minified React error #310」这种编号，光看编号猜不出是哪行。
 * 所以这里把 error.stack 也显示出来 —— 用户截个图发过来，
 * 就能定位到具体组件和行号。
 */
class PageBoundary extends Component<{ children: ReactNode }, { err: Error | null }> {
  state = { err: null as Error | null }

  static getDerivedStateFromError(err: Error) {
    return { err }
  }

  render() {
    if (this.state.err) {
      const e = this.state.err
      const stack = e && e.stack ? String(e.stack).split('\n').slice(0, 6).join('\n') : ''
      return (
        <div className="m-4 rounded-xl bg-rose-50 p-4 ring-1 ring-rose-200">
          <p className="text-sm font-bold text-rose-700">这个页面出错了，其它页面不受影响</p>

          <p className="mt-2 break-all text-xs font-semibold text-rose-700">
            {String(e && e.message ? e.message : e)}
          </p>

          {stack && (
            <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all rounded bg-white/70 p-2 font-mono text-[10px] leading-relaxed text-rose-600">
              {stack}
            </pre>
          )}

          <p className="mt-2 text-[11px] text-rose-500">
            把这个红框截图发给开发者，就能定位到具体是哪一行。
          </p>

          <button
            onClick={() => this.setState({ err: null })}
            className="mt-3 rounded-md bg-rose-500 px-3 py-1 text-xs font-semibold text-white hover:bg-rose-600"
          >
            重试
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

export default PageBoundary
