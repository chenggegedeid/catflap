import { Component, type ReactNode } from 'react'

/**
 * 页面级错误边界。
 *
 * **为什么需要**：任何一个页面渲染时抛异常，如果没有边界兜底，
 * React 会把整棵树卸载 —— 表现是**全站白屏**（连侧栏都没了），
 * 控制台里只有一句压缩过的报错，用户说不清、我们也难定位。
 *
 * 套上之后：崩的只是那一页，其它页面照常；错误原文直接显示在页面上，
 * 用户截个图我们就能定位。
 */
class PageBoundary extends Component<{ children: ReactNode }, { err: Error | null }> {
  state = { err: null as Error | null }

  static getDerivedStateFromError(err: Error) {
    return { err }
  }

  render() {
    if (this.state.err) {
      return (
        <div className="m-4 rounded-xl bg-rose-50 p-4 ring-1 ring-rose-200">
          <p className="text-sm font-bold text-rose-700">这个页面出错了，其它页面不受影响</p>
          <p className="mt-2 break-all font-mono text-xs text-rose-600">
            {String(this.state.err && this.state.err.message ? this.state.err.message : this.state.err)}
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
