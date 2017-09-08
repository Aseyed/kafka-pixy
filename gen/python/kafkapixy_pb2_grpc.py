# Generated by the gRPC Python protocol compiler plugin. DO NOT EDIT!
import grpc
from grpc.framework.common import cardinality
from grpc.framework.interfaces.face import utilities as face_utilities

import kafkapixy_pb2 as kafkapixy__pb2


class KafkaPixyStub(object):

  def __init__(self, channel):
    """Constructor.

    Args:
      channel: A grpc.Channel.
    """
    self.Produce = channel.unary_unary(
        '/KafkaPixy/Produce',
        request_serializer=kafkapixy__pb2.ProdRq.SerializeToString,
        response_deserializer=kafkapixy__pb2.ProdRs.FromString,
        )
    self.ConsumeNAck = channel.unary_unary(
        '/KafkaPixy/ConsumeNAck',
        request_serializer=kafkapixy__pb2.ConsNAckRq.SerializeToString,
        response_deserializer=kafkapixy__pb2.ConsRs.FromString,
        )
    self.Ack = channel.unary_unary(
        '/KafkaPixy/Ack',
        request_serializer=kafkapixy__pb2.AckRq.SerializeToString,
        response_deserializer=kafkapixy__pb2.AckRs.FromString,
        )
    self.GetOffsets = channel.unary_unary(
        '/KafkaPixy/GetOffsets',
        request_serializer=kafkapixy__pb2.GetOffsetsRq.SerializeToString,
        response_deserializer=kafkapixy__pb2.GetOffsetsRs.FromString,
        )


class KafkaPixyServicer(object):

  def Produce(self, request, context):
    """Produce writes a message to a Kafka topic.

    If ProdReq.async_mode is false (default value) then the request will
    block until the message is written to all ISR. In this case the respose
    will contain the partition and offset of the message. This has to be
    used to achive at-least-once deliverability guarantee.
    If ProdReq.async_mode is true, then Kafka-Pixy returns immediately after
    it gets the request and performs write on the backgroud. This mode
    ensures highest throughput but messages can be lost, e.g. if the host
    crashes before Kafka-Pixy has a chance to complete write.

    Hash of ProdReq.key_value is used to determine a partition that the
    message should be written to. If you want a message to go to an random
    partition then set ProdReq.key_undefined to true. Note that if both
    ProdReq.key_undefined and ProdReq.key_value are left default, which is
    empty string and false respectively, then messages will be consitently
    written to a partiticular partition selected by the hash of an empty
    string.

    gRPC error codes:
    * Invalid Argument (3): see the status description for details;
    * Internal (13): see the status description and logs for details;
    * Unavailable (14): the service is shutting down.
    """
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')

  def ConsumeNAck(self, request, context):
    """Consume reads a message from a topic and optionally acknowledges a
    message previously consumed from the same topic.

    Requests are performed in long polling fation, that is if all available
    messages have been consumed then the request will block for
    config.yaml:proxies.<cluster>.consumer.long_polling_timeout waiting for
    new messages. If no new messages is produced while waiting the request
    will return gRPC error with 408 status code.

    To consume the first message set ConsNAckReq.no_ack to true, since there
    is no message to acknowledge at this point. In the second and all
    subsequent calls of the method set ConsNAckReq.ack_partition and
    ConsNAckReq.ack_offset to the respective values of ConsRes returned by
    the previous method call. To acknowledge the last consumed message before
    teminating the application call Ack method.

    If a message is not acknowledged within
    config.yaml:proxies.<cluster>.consumer.ack_timeout the it will be returned
    by Kafka-Pixy in ConsRes again possibly to another application.

    If at-least-once delivery guarantee and retries are not desirable, then
    you can set ConsNAckReq.auto_ack to true and Kafka-Pixy will acknowledge
    messages automatically before returning them in ConsRes.

    gRPC error codes:
    * Not Found (5): It just means that all message has been consumed and
    the long polling timeout has elaspsed. Just keep calling this method
    in a loop;
    * Resource Exhausted (8): too many consume requests. Either reduce the
    number of consuming threads or increase
    config.yaml:proxies.<cluster>.consumer.channel_buffer_size;
    * Invalid Argument (3): see the status description for details;
    * Internal (13): see the status description and logs for details;
    * Unavailable (14): the service is shutting down.
    """
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')

  def Ack(self, request, context):
    """Ack acknowledges a message earlier consumed from a topic.

    This method is provided solely to acknowledge the last consumed message
    before the application terminates. In all other cases ConsumeNAck should
    be used.

    gRPC error codes:
    * Invalid Argument (3): see the status description for details;
    * Internal (13): see the status description and logs for details;
    """
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')

  def GetOffsets(self, request, context):
    """Fetches partition offsets for the specified topic and group

    gRPC error codes:
    * Invalid Argument (3): If unable to find the cluster named in the request
    * Internal (13): If Kafka returns an error on offset request
    * NotFound (5): If the group and or topic does not exist
    """
    context.set_code(grpc.StatusCode.UNIMPLEMENTED)
    context.set_details('Method not implemented!')
    raise NotImplementedError('Method not implemented!')


def add_KafkaPixyServicer_to_server(servicer, server):
  rpc_method_handlers = {
      'Produce': grpc.unary_unary_rpc_method_handler(
          servicer.Produce,
          request_deserializer=kafkapixy__pb2.ProdRq.FromString,
          response_serializer=kafkapixy__pb2.ProdRs.SerializeToString,
      ),
      'ConsumeNAck': grpc.unary_unary_rpc_method_handler(
          servicer.ConsumeNAck,
          request_deserializer=kafkapixy__pb2.ConsNAckRq.FromString,
          response_serializer=kafkapixy__pb2.ConsRs.SerializeToString,
      ),
      'Ack': grpc.unary_unary_rpc_method_handler(
          servicer.Ack,
          request_deserializer=kafkapixy__pb2.AckRq.FromString,
          response_serializer=kafkapixy__pb2.AckRs.SerializeToString,
      ),
      'GetOffsets': grpc.unary_unary_rpc_method_handler(
          servicer.GetOffsets,
          request_deserializer=kafkapixy__pb2.GetOffsetsRq.FromString,
          response_serializer=kafkapixy__pb2.GetOffsetsRs.SerializeToString,
      ),
  }
  generic_handler = grpc.method_handlers_generic_handler(
      'KafkaPixy', rpc_method_handlers)
  server.add_generic_rpc_handlers((generic_handler,))