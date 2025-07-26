import './App.css'
import DonorDashboard from "@/components/DonorDashboard";
import CampaignLandingPage from '@/components/CampaignLandingPage';

function App() {
  return (
    <div className="flex gap-12 flex-col min-h-screen">
      <div className="min-h-[50vh]">
        <CampaignLandingPage />
      </div>
      <div className="min-h-[50vh]">
        <DonorDashboard />
      </div>
    </div>
  )
}

export default App
