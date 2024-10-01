import grpc
import kafka_tests.price_service_pb2 as price_service_pb2
import kafka_tests.price_service_pb2_grpc as price_service_pb2_grpc
import asyncio
import sys
import logging
import time

logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)

RETRY_DELAY = .1  # seconds

async def stream_prices():
    while True:
        try:
            # Create a gRPC channel
            channel = grpc.aio.insecure_channel('localhost:50051')
            
            # Create a stub (client)
            stub = price_service_pb2_grpc.PriceServiceStub(channel)

            # Create a request
            request = price_service_pb2.PriceRequest()

            # Call the StreamPrices method and iterate over the responses
            async for response in stub.StreamPrices(request):
                logger.info(f"Received: Timestamp: {response.timestamp}, "
                      f"Price: {response.price}, "
                      f"Confidence Interval: {response.confidence_interval}, "
                      f"Status: {response.status}")

        except grpc.aio.AioRpcError as e:
            logger.error(f"RPC error: {e}")
            logger.info(f"Attempting to reconnect in {RETRY_DELAY} seconds...")
            await asyncio.sleep(RETRY_DELAY)
        
        except Exception as e:
            logger.error(f"Unexpected error: {e}")
            logger.info(f"Attempting to reconnect in {RETRY_DELAY} seconds...")
            await asyncio.sleep(RETRY_DELAY)

async def run():
    while True:
        try:
            await stream_prices()
        except KeyboardInterrupt:
            logger.info("Client stopped by user.")
            break
        except Exception as e:
            logger.error(f"Unexpected error in main loop: {e}")
            await asyncio.sleep(RETRY_DELAY)

if __name__ == '__main__':
    if sys.platform.startswith('win'):
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())
    asyncio.run(run())