mod gatewaypb {
    tonic::include_proto!("gateway");
}

use gatewaypb::gateway_client;
use tonic::transport::channel::Channel;

pub type GatewayClient = gateway_client::GatewayClient<Channel>;
pub use gatewaypb::{EmptyRequest, StatsResponse};

async fn test() -> Result<Vec<GatewayClient>, Box<dyn std::error::Error>> {
    let mut conn = GatewayClient::connect("asdf").await?;
    let res = conn.version(gatewaypb::EmptyRequest {}).await?.into_inner();
    println!("{}", res.version);
    Ok(vec![conn])
}
