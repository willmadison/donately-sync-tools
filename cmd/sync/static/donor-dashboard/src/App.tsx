import './App.css'
import DonorDashboard from "@/components/DonorDashboard";
import DonateNow from '@/components/DonateNow';

function App() {
  return (
    <div className="flex gap-12 flex-col min-h-screen">
      <DonateNow />
      <div className="min-h-[50vh]">
        <DonorDashboard />
      </div>
    </div>
  )
}

export default App
