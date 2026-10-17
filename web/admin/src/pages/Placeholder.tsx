import { Construction } from 'lucide-react'
import { Card } from '../components/Card'

export function Placeholder({ name }: { name: string }) {
  return (
    <Card>
      <div className="grid place-items-center py-24 text-center">
        <div className="grid h-14 w-14 place-items-center rounded-2xl bg-surface-2 text-muted">
          <Construction className="h-7 w-7" />
        </div>
        <div className="mt-4 text-base font-bold text-content">{name}</div>
        <div className="mt-1 text-sm text-muted">该模块尚未实现，敬请期待</div>
      </div>
    </Card>
  )
}
