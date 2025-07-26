import { Card, CardContent } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Badge } from "@/components/ui/badge";
import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";

interface CampaignOverview {
    id: string;
    title: string;
    slug: string;
    type: string;
    url: string;
    status: string;
    permalink: string;
    description: string | null;
    content: string | null;
    created: number;
    updated: number;
    start_date: string | null;
    end_date: string | null;
    goal_in_cents: number;
    amount_raised_in_cents: number;
    percent_funded: number;
    donors: Donor[];
}

interface Donor {
    person: Person;
    pledge: number;
    donations: Donation[];
    adjustments: Adjustment[];
}

export interface Person {
    id: string;
    first_name: string;
    last_name: string;
}

export interface Donation {
    id: string;
    amount_in_cents: number;
}

export interface Adjustment {
    name: string;
    slug: string;
    amount: number;
}


export default function DonorDashboard() {
    const [campaignOverview, setCampaignOverview] = useState<CampaignOverview | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        const fetchCampaignOverview = async () => {
            try {
                const res = await fetch("/api/campaign/overview");
                if (!res.ok) throw new Error(`HTTP error! status: ${res.status}`);
                const data = await res.json();
                setCampaignOverview(data);
            } catch (err: any) {
                setError(err.message);
            } finally {
                setLoading(false);
            }
        };

        fetchCampaignOverview();
    }, []);

    if (loading) return <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />;
    if (error) return <div className="text-red-500">Error: {error}</div>;

    if (!campaignOverview) return <div>No Campaign Info</div>;

    const totalGoal = campaignOverview.goal_in_cents / 100;
    const totalDonated = campaignOverview.amount_raised_in_cents / 100;
    const overallProgress = Math.min((totalDonated / totalGoal) * 100, 100);

    return (
        <div className="p-6 space-y-6">
            <h1 className="text-2xl font-bold">Campaign Progress</h1>
            <Card>
                <CardContent className="p-4">
                    <div className="flex items-center justify-between mb-2">
                        <span>Total Raised: ${totalDonated} / ${totalGoal}</span>
                        <span>{Math.round(overallProgress)}%</span>
                    </div>
                    <Progress value={overallProgress} />
                </CardContent>
            </Card>

            <h2 className="text-xl font-semibold">Donor Progress</h2>
            <div className="space-y-4 max-h-[400px] overflow-y-auto pr-2">
                {campaignOverview.donors.map((donor, index) => {
                    const totalDonations = donor.donations.reduce((sum, donation) => sum + (donation.amount_in_cents / 100), 0);
                    const totalAdjustments = donor.adjustments.reduce((sum, adjustment) => sum + adjustment.amount, 0);
                    const goalMet = totalDonations + totalAdjustments >= donor.pledge;
                    const progress = Math.min(((totalDonations + totalAdjustments) / donor.pledge) * 100, 100);

                    return (
                        <Card
                            key={index}
                            className={`transition-shadow hover:shadow-md cursor-pointer ${goalMet ? "border-green-500" : ""
                                }`}
                        >
                            <CardContent className="p-4 space-y-2">
                                <div className="flex justify-between items-center">
                                    <h3 className="font-medium text-lg">{donor.person.first_name}&nbsp;{donor.person.last_name}</h3>
                                    {goalMet && <Badge variant="default">Goal Met</Badge>}
                                </div>
                                <div className="flex justify-between text-sm">
                                    <span>${(totalDonations + totalAdjustments).toFixed(2)} / ${donor.pledge}</span>
                                    <span>{Math.round(progress)}%</span>
                                </div>
                                <Progress value={progress} />
                            </CardContent>
                        </Card>
                    );
                })}
            </div>
        </div>
    );
}